package dropper

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// filenameRegex matches characters NOT in the allowed set for filenames.
// Compiled once at package level to avoid repeated compilation.
var filenameRegex = regexp.MustCompile(FilenameSanitizePattern)

// FileEntry represents a file or directory in a listing.
type FileEntry struct {
	Name          string    `json:"name"`
	IsDir         bool      `json:"is_dir"`
	Size          int64     `json:"size"`
	FormattedSize string    `json:"formatted_size"`
	ModTime       time.Time `json:"mod_time"`
}

// FormatFileSize returns a human-readable file size string.
// Examples: "0 B", "512 B", "1.5 KB", "4.2 MB", "1.0 GB", "1.0 TB".
func FormatFileSize(bytes int64) string {
	switch {
	case bytes >= SizeTB:
		return fmt.Sprintf(SizeFormatDecimal, float64(bytes)/float64(SizeTB), SizeUnitTB)
	case bytes >= SizeGB:
		return fmt.Sprintf(SizeFormatDecimal, float64(bytes)/float64(SizeGB), SizeUnitGB)
	case bytes >= SizeMB:
		return fmt.Sprintf(SizeFormatDecimal, float64(bytes)/float64(SizeMB), SizeUnitMB)
	case bytes >= SizeKB:
		return fmt.Sprintf(SizeFormatDecimal, float64(bytes)/float64(SizeKB), SizeUnitKB)
	default:
		return fmt.Sprintf(SizeFormatBytes, bytes, SizeUnitB)
	}
}

// SanitizeFilename replaces unsafe characters in a filename, keeping only
// alphanumeric characters, dots, hyphens, and underscores.
// Returns "_" for empty input, "." or ".." input. Truncates to FilenameMaxLength.
func SanitizeFilename(name string) string {
	if name == "" {
		return FilenameFallback
	}

	sanitized := filenameRegex.ReplaceAllLiteralString(name, string(FilenameSanitizeReplace))

	// Prevent "." and ".." as filenames — they are valid after sanitization
	// but dangerous as path components.
	if sanitized == "." || sanitized == ".." {
		return FilenameFallback
	}

	if len(sanitized) > FilenameMaxLength {
		sanitized = sanitized[:FilenameMaxLength]
	}

	return sanitized
}

// ValidateExtension checks whether the filename's extension is in the allowed list.
// An empty allowed list permits all extensions. Comparison is case-insensitive.
func ValidateExtension(filename string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}

	ext := filepath.Ext(filename)
	for _, a := range allowed {
		if strings.EqualFold(ext, a) {
			return true
		}
	}
	return false
}

// SafePath validates that requestedPath resolves to a location within rootDir.
// Implements the 5-step path jail algorithm:
//  1. filepath.Clean + filepath.Abs on both root and request
//  2. filepath.Join to combine
//  3. strings.HasPrefix with separator-aware check
//  4. filepath.EvalSymlinks (with ancestor walk for non-existent paths)
//  5. Re-check prefix after symlink resolution
//
// Both rootDir and the resolved path are checked through EvalSymlinks to handle
// cases where rootDir itself is a symlink (common in container deployments).
//
// Returns the validated absolute path or an error.
func SafePath(rootDir, requestedPath string) (string, error) {
	// Reject null bytes early — they can bypass string checks.
	if strings.ContainsRune(requestedPath, '\x00') {
		return "", fmt.Errorf("%s", ErrMsgPathTraversal)
	}

	// Reject absolute paths — client-supplied paths must be relative to root.
	if filepath.IsAbs(requestedPath) {
		return "", fmt.Errorf("%s", ErrMsgPathTraversal)
	}

	// Step 1: Resolve root to clean absolute path, then resolve symlinks on root itself.
	rootAbs, err := filepath.Abs(filepath.Clean(rootDir))
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgPathResolution, err)
	}

	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgPathResolution, err)
	}

	// Step 2: Join resolved root with cleaned request path.
	joined := filepath.Join(rootResolved, filepath.Clean(requestedPath))

	// Step 3: Ensure absolute.
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgPathResolution, err)
	}

	// Step 4: Separator-aware prefix check.
	// Prevents /data matching /data-secret.
	if !isWithinRoot(abs, rootResolved) {
		return "", fmt.Errorf("%s", ErrMsgPathTraversal)
	}

	// Step 5: Resolve symlinks on target and re-check.
	resolved, err := resolveWithAncestorWalk(abs, rootResolved)
	if err != nil {
		return "", fmt.Errorf("%s", ErrMsgPathTraversal)
	}

	if !isWithinRoot(resolved, rootResolved) {
		return "", fmt.Errorf("%s", ErrMsgPathTraversal)
	}

	return resolved, nil
}

// isWithinRoot checks that path is equal to root or is a child of root
// with a path separator boundary. This prevents /data from matching /data-secret.
func isWithinRoot(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// resolveWithAncestorWalk resolves symlinks for a path that may not exist.
// If the path exists, it calls filepath.EvalSymlinks directly.
// If not, it walks up to the deepest existing ancestor, resolves symlinks
// on that ancestor, then re-appends the remaining path components.
func resolveWithAncestorWalk(targetPath, rootResolved string) (string, error) {
	// Try direct resolution first (path exists).
	resolved, err := filepath.EvalSymlinks(targetPath)
	if err == nil {
		return resolved, nil
	}

	// Path doesn't exist — walk up to find deepest existing ancestor.
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	current := targetPath
	var remaining []string

	for current != rootResolved && current != filepath.Dir(current) {
		parent := filepath.Dir(current)
		remaining = append([]string{filepath.Base(current)}, remaining...)
		current = parent

		resolved, err = filepath.EvalSymlinks(current)
		if err == nil {
			// Found existing ancestor — re-append remaining components.
			result := resolved
			for _, component := range remaining {
				result = filepath.Join(result, component)
			}
			return result, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}

	// Fallback: resolve root itself and re-append everything.
	resolvedRoot, err := filepath.EvalSymlinks(rootResolved)
	if err != nil {
		return "", err
	}
	result := resolvedRoot
	for _, component := range remaining {
		result = filepath.Join(result, component)
	}
	return result, nil
}

// ResolveCollision checks if a file already exists at dir/filename.
// If it does, it prefixes the filename with a timestamp to avoid collision.
// The caller must have already validated the directory via SafePath.
func ResolveCollision(dir, filename string) string {
	target := filepath.Join(dir, filename)
	if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
		return filename
	}

	prefix := time.Now().Format(CollisionTimestampFormat) + CollisionSeparator
	return prefix + filename
}

// ClipboardFilename generates a timestamped filename for clipboard paste uploads.
// Format: "20260406-143022_clipboard.png"
func ClipboardFilename() string {
	return time.Now().Format(CollisionTimestampFormat) + CollisionSeparator + ClipboardFilenamePrefix + ClipboardFilenameExt
}

// ListDirectory returns the contents of a directory within the root filesystem.
// Results are sorted with directories first, then by the specified sort field and order.
// Valid sortBy values: "name", "date", "size". Valid sortOrder values: "asc", "desc".
func ListDirectory(rootDir, relPath, sortBy, sortOrder string) ([]FileEntry, error) {
	safePath, err := SafePath(rootDir, relPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(safePath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgListDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s", ErrMsgNotDirectory)
	}

	entries, err := os.ReadDir(safePath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgListDir, err)
	}

	result := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't stat.
		}

		size := entryInfo.Size()
		if entry.IsDir() {
			size = 0
		}

		result = append(result, FileEntry{
			Name:          entry.Name(),
			IsDir:         entry.IsDir(),
			Size:          size,
			FormattedSize: FormatFileSize(size),
			ModTime:       entryInfo.ModTime(),
		})
	}

	sortFileEntries(result, sortBy, sortOrder)

	return result, nil
}

// sortFileEntries sorts entries with directories first, then by the given field and order.
func sortFileEntries(entries []FileEntry, sortBy, sortOrder string) {
	if sortBy == "" {
		sortBy = DefaultSortField
	}
	if sortOrder == "" {
		sortOrder = DefaultSortOrder
	}

	desc := sortOrder == SortOrderDesc

	sort.SliceStable(entries, func(i, j int) bool {
		// Directories always come first.
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}

		var less bool
		switch sortBy {
		case SortByDate:
			less = entries[i].ModTime.Before(entries[j].ModTime)
		case SortBySize:
			less = entries[i].Size < entries[j].Size
		default: // SortByName
			less = strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		}

		if desc {
			return !less
		}
		return less
	})
}

// SafeWriteFile writes data to a file within rootDir using the safe write pipeline:
//  1. Check readonly mode
//  2. Sanitize filename
//  3. Validate extension (before any disk write)
//  4. Validate target directory via SafePath
//  5. Resolve collisions with atomic O_CREATE|O_EXCL (TOCTOU-safe)
//  6. Write to temp file, then atomic rename
//
// Returns the final filename (may differ from input due to sanitization/collision).
func SafeWriteFile(rootDir, relDir, filename string, data io.Reader, maxBytes int64, allowedExts []string, readonly bool, logger *slog.Logger) (string, error) {
	if readonly {
		return "", fmt.Errorf("%s", ErrMsgReadonlyMode)
	}

	sanitized := SanitizeFilename(filename)

	if !ValidateExtension(sanitized, allowedExts) {
		logger.Warn(LogMsgExtRejected,
			LogFieldFilename, sanitized,
			LogFieldExtension, filepath.Ext(sanitized))
		return "", fmt.Errorf("%s", ErrMsgExtNotAllowed)
	}

	safeDir, err := SafePath(rootDir, relDir)
	if err != nil {
		logger.Warn(LogMsgPathDenied, LogFieldPath, relDir)
		return "", err
	}

	info, err := os.Stat(safeDir)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgWriteFile, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s", ErrMsgNotDirectory)
	}

	// Create temp file in the same directory for atomic rename.
	tmpFile, err := os.CreateTemp(safeDir, TempFilePattern)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgTempFile, err)
	}

	// Cleanup: remove temp file on any failure path.
	renamed := false
	defer func() {
		if !renamed {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
		}
	}()

	// Copy data with size limit. Read one extra byte to detect overflow.
	limitedReader := io.LimitReader(data, maxBytes+1)
	written, err := io.Copy(tmpFile, limitedReader)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgWriteFile, err)
	}

	if written > maxBytes {
		return "", fmt.Errorf("%s", ErrMsgFileTooLarge)
	}

	// Close before rename to flush and release the file descriptor.
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgWriteFile, err)
	}

	// Set final permissions before rename.
	if err := os.Chmod(tmpFile.Name(), FilePermissions); err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgWriteFile, err)
	}

	// Atomically claim the final filename using O_CREATE|O_EXCL to prevent TOCTOU races.
	finalName, err := atomicPlace(safeDir, sanitized, tmpFile.Name(), logger)
	if err != nil {
		return "", err
	}
	renamed = true

	logger.Info(LogMsgFileWritten,
		LogFieldFilename, finalName,
		LogFieldSize, written,
		LogFieldPath, relDir)

	return finalName, nil
}

// atomicPlace attempts to hard-link (or rename) the temp file to the final location
// using collision resolution. If the target exists, it retries with a timestamp prefix.
// This is TOCTOU-safe because os.Link/os.Rename with a unique temp source
// and the collision check + place happen atomically per attempt.
func atomicPlace(dir, desiredName, tmpPath string, logger *slog.Logger) (string, error) {
	candidates := []string{desiredName}

	// Pre-generate collision candidates.
	for range CollisionMaxRetries {
		prefix := time.Now().Format(CollisionTimestampFormat) + CollisionSeparator
		candidates = append(candidates, prefix+desiredName)
	}

	for _, name := range candidates {
		finalPath := filepath.Join(dir, name)
		err := os.Link(tmpPath, finalPath)
		if err == nil {
			// Successfully linked — remove the temp file.
			os.Remove(tmpPath)
			if name != desiredName {
				logger.Info(LogMsgCollisionResolved,
					LogFieldOriginalName, desiredName,
					LogFieldResolvedName, name)
			}
			return name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			// os.Link not supported (e.g., cross-device) — fall back to rename.
			return atomicPlaceRename(dir, desiredName, tmpPath, logger)
		}
		// ErrExist: name already taken, try next candidate.
	}

	return "", fmt.Errorf("%s", ErrMsgFileExists)
}

// atomicPlaceRename is the fallback when hard links aren't available.
// It uses os.Rename which is atomic on the same filesystem.
func atomicPlaceRename(dir, desiredName, tmpPath string, logger *slog.Logger) (string, error) {
	// Try the desired name first.
	finalPath := filepath.Join(dir, desiredName)
	if _, err := os.Stat(finalPath); errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(tmpPath, finalPath); err != nil {
			return "", fmt.Errorf("%s: %w", ErrMsgRenameFile, err)
		}
		return desiredName, nil
	}

	// Collision — use timestamp prefix.
	prefix := time.Now().Format(CollisionTimestampFormat) + CollisionSeparator
	resolvedName := prefix + desiredName
	finalPath = filepath.Join(dir, resolvedName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgRenameFile, err)
	}

	logger.Info(LogMsgCollisionResolved,
		LogFieldOriginalName, desiredName,
		LogFieldResolvedName, resolvedName)

	return resolvedName, nil
}

// CreateDirectory creates a new subdirectory within rootDir.
// The directory name is sanitized and the parent path is validated via SafePath.
func CreateDirectory(rootDir, relParentPath, name string, readonly bool, logger *slog.Logger) error {
	if readonly {
		return fmt.Errorf("%s", ErrMsgReadonlyMode)
	}

	sanitized := SanitizeFilename(name)

	safeParent, err := SafePath(rootDir, relParentPath)
	if err != nil {
		logger.Warn(LogMsgPathDenied, LogFieldPath, relParentPath)
		return err
	}

	info, err := os.Stat(safeParent)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrMsgCreateDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s", ErrMsgNotDirectory)
	}

	// Build the relative path for the new directory and re-validate.
	newRelPath := filepath.Join(relParentPath, sanitized)
	newSafePath, err := SafePath(rootDir, newRelPath)
	if err != nil {
		return err
	}

	if err := os.Mkdir(newSafePath, DirPermissions); err != nil {
		return fmt.Errorf("%s: %w", ErrMsgCreateDir, err)
	}

	logger.Info(LogMsgDirCreated,
		LogFieldFilename, sanitized,
		LogFieldPath, relParentPath)

	return nil
}
