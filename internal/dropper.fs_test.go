package dropper

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// safeFilenamePattern validates sanitized filenames contain only allowed characters.
// Compiled once at package level per reviewer finding #9.
var safeFilenamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)

// --- FormatFileSize tests ---

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero bytes", bytes: 0, want: "0 B"},
		{name: "small bytes", bytes: 512, want: "512 B"},
		{name: "exactly 1 KB", bytes: SizeKB, want: "1.0 KB"},
		{name: "1.5 KB", bytes: 1536, want: "1.5 KB"},
		{name: "exactly 1 MB", bytes: SizeMB, want: "1.0 MB"},
		{name: "4.2 MB", bytes: 4404019, want: "4.2 MB"},
		{name: "exactly 1 GB", bytes: SizeGB, want: "1.0 GB"},
		{name: "exactly 1 TB", bytes: SizeTB, want: "1.0 TB"},
		{name: "318 KB", bytes: 325632, want: "318.0 KB"},
		{name: "1 byte", bytes: 1, want: "1 B"},
		{name: "1023 bytes", bytes: 1023, want: "1023 B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatFileSize(tt.bytes)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- SanitizeFilename tests ---

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "normal filename", input: "hello.txt", want: "hello.txt"},
		{name: "spaces replaced", input: "file name.txt", want: "file_name.txt"},
		{name: "path separators replaced", input: "../../etc/passwd", want: ".._.._etc_passwd"},
		{name: "unicode replaced", input: "café.png", want: "caf_.png"},
		{name: "empty string", input: "", want: FilenameFallback},
		{name: "dots allowed", input: "...", want: "..."},
		{name: "single dot becomes fallback", input: ".", want: FilenameFallback},
		{name: "double dot becomes fallback", input: "..", want: FilenameFallback},
		{name: "dash allowed", input: "-file.txt", want: "-file.txt"},
		{name: "special chars replaced", input: "hello world!@#.jpg", want: "hello_world___.jpg"},
		{name: "null byte replaced", input: "\x00evil", want: "_evil"},
		{name: "underscore preserved", input: "my_file_v2.tar.gz", want: "my_file_v2.tar.gz"},
		{name: "all unsafe chars", input: "~`!@#$%^&*()+=[]{}", want: "__________________"},
		{name: "only path separators", input: "////", want: "____"},
		{name: "backslashes", input: `a\b\c`, want: "a_b_c"},
		{
			name:  "long filename truncated",
			input: strings.Repeat("a", 300),
			want:  strings.Repeat("a", FilenameMaxLength),
		},
		{name: "mixed safe and unsafe", input: "report-2026.Q1 (final).pdf", want: "report-2026.Q1__final_.pdf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			assert.Equal(t, tt.want, got)
			// Verify result only contains safe characters.
			assert.True(t, safeFilenamePattern.MatchString(got),
				"sanitized filename contains unsafe characters: %q", got)
		})
	}
}

// --- ValidateExtension tests ---

func TestValidateExtension(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		allowed  []string
		want     bool
	}{
		{name: "allowed extension", filename: "file.png", allowed: []string{".png", ".jpg"}, want: true},
		{name: "disallowed extension", filename: "file.gif", allowed: []string{".png", ".jpg"}, want: false},
		{name: "case insensitive match", filename: "file.PNG", allowed: []string{".png"}, want: true},
		{name: "empty list allows all", filename: "file.txt", allowed: []string{}, want: true},
		{name: "nil list allows all", filename: "file.txt", allowed: nil, want: true},
		{name: "no extension disallowed", filename: "noext", allowed: []string{".png"}, want: false},
		{name: "double extension checks last", filename: "file.tar.gz", allowed: []string{".gz"}, want: true},
		{name: "no extension empty list", filename: "file", allowed: []string{}, want: true},
		{name: "hidden file as extension", filename: ".hidden", allowed: []string{".hidden"}, want: true},
		{name: "mixed case allowed list", filename: "file.Jpg", allowed: []string{".JPG"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateExtension(tt.filename, tt.allowed)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- SafePath tests ---

func TestSafePath(t *testing.T) {
	root := t.TempDir()

	// Create test directory structure.
	subdir := filepath.Join(root, "subdir")
	require.NoError(t, os.MkdirAll(subdir, DirPermissions))

	nested := filepath.Join(root, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, DirPermissions))

	// Create a symlink pointing inside root.
	symlinkInside := filepath.Join(root, "link-inside")
	require.NoError(t, os.Symlink(subdir, symlinkInside))

	// Create a symlink pointing outside root.
	outsideDir := t.TempDir()
	symlinkOutside := filepath.Join(root, "link-outside")
	require.NoError(t, os.Symlink(outsideDir, symlinkOutside))

	rootAbs, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	tests := []struct {
		name      string
		requested string
		wantErr   bool
		wantPath  string // Expected path prefix or exact match (empty = don't check).
	}{
		{name: "valid subdir", requested: "subdir", wantErr: false, wantPath: filepath.Join(rootAbs, "subdir")},
		{name: "valid nested", requested: "a/b/c", wantErr: false, wantPath: filepath.Join(rootAbs, "a", "b", "c")},
		{name: "root itself empty", requested: "", wantErr: false, wantPath: rootAbs},
		{name: "root itself dot", requested: ".", wantErr: false, wantPath: rootAbs},
		{name: "relative traversal", requested: "../etc/passwd", wantErr: true},
		{name: "double dot in middle", requested: "a/../../etc", wantErr: true},
		{name: "absolute path outside", requested: "/etc/passwd", wantErr: true},
		{name: "symlink escape", requested: "link-outside", wantErr: true},
		{name: "symlink inside root", requested: "link-inside", wantErr: false, wantPath: filepath.Join(rootAbs, "subdir")},
		{name: "null byte", requested: "file\x00.txt", wantErr: true},
		{name: "deeply nested valid non-existent", requested: "a/b/c/d/e/f", wantErr: false},
		{name: "trailing slash", requested: "subdir/", wantErr: false, wantPath: filepath.Join(rootAbs, "subdir")},
		{name: "dot segments normalized", requested: "./subdir/../subdir", wantErr: false, wantPath: filepath.Join(rootAbs, "subdir")},
		{name: "non-existent target in root", requested: "newdir/newfile", wantErr: false},
		{name: "traversal with nested dots", requested: "subdir/../../..", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafePath(root, tt.requested)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), ErrMsgPathTraversal)
				// Verify no path info leaks in error message.
				assert.NotContains(t, err.Error(), root)
			} else {
				require.NoError(t, err)
				assert.True(t, isWithinRoot(got, rootAbs),
					"result %q should be within root %q", got, rootAbs)
				if tt.wantPath != "" {
					assert.Equal(t, tt.wantPath, got)
				}
			}
		})
	}
}

func TestSafePath_RootPrefixCollision(t *testing.T) {
	// Create two directories that share a prefix: /tmp/.../data and /tmp/.../data-secret
	parent := t.TempDir()
	root := filepath.Join(parent, "data")
	secret := filepath.Join(parent, "data-secret")
	require.NoError(t, os.MkdirAll(root, DirPermissions))
	require.NoError(t, os.MkdirAll(secret, DirPermissions))

	// A symlink within root pointing to data-secret should be caught.
	symlinkPath := filepath.Join(root, "escape")
	require.NoError(t, os.Symlink(secret, symlinkPath))

	_, err := SafePath(root, "escape")
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgPathTraversal)
}

func TestSafePath_SymlinkRoot(t *testing.T) {
	// Test that SafePath works when rootDir itself is a symlink.
	realRoot := t.TempDir()
	subdir := filepath.Join(realRoot, "subdir")
	require.NoError(t, os.MkdirAll(subdir, DirPermissions))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("data"), FilePermissions))

	// Create a symlink to realRoot.
	symlinkRoot := filepath.Join(t.TempDir(), "symlink-root")
	require.NoError(t, os.Symlink(realRoot, symlinkRoot))

	// SafePath through the symlinked root should work.
	got, err := SafePath(symlinkRoot, "subdir/file.txt")
	require.NoError(t, err)

	realRootResolved, err := filepath.EvalSymlinks(realRoot)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(realRootResolved, "subdir", "file.txt"), got)
}

// --- ResolveCollision tests ---

func TestResolveCollision(t *testing.T) {
	dir := t.TempDir()

	t.Run("no conflict returns original", func(t *testing.T) {
		got := ResolveCollision(dir, "unique.txt")
		assert.Equal(t, "unique.txt", got)
	})

	t.Run("file conflict adds timestamp prefix", func(t *testing.T) {
		existing := filepath.Join(dir, "existing.txt")
		require.NoError(t, os.WriteFile(existing, []byte("content"), FilePermissions))

		got := ResolveCollision(dir, "existing.txt")
		assert.NotEqual(t, "existing.txt", got)
		assert.True(t, strings.HasSuffix(got, CollisionSeparator+"existing.txt"),
			"resolved name %q should end with _existing.txt", got)
		// Verify the new name doesn't exist yet.
		_, err := os.Stat(filepath.Join(dir, got))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("directory conflict adds timestamp prefix", func(t *testing.T) {
		existingDir := filepath.Join(dir, "existingdir")
		require.NoError(t, os.Mkdir(existingDir, DirPermissions))

		got := ResolveCollision(dir, "existingdir")
		assert.NotEqual(t, "existingdir", got)
		assert.True(t, strings.HasSuffix(got, CollisionSeparator+"existingdir"))
	})

	t.Run("resolved name matches timestamp pattern", func(t *testing.T) {
		conflict := filepath.Join(dir, "conflict.png")
		require.NoError(t, os.WriteFile(conflict, []byte("data"), FilePermissions))

		got := ResolveCollision(dir, "conflict.png")
		pattern := regexp.MustCompile(`^\d{8}-\d{6}_conflict\.png$`)
		assert.True(t, pattern.MatchString(got),
			"resolved name %q should match YYYYMMDD-HHmmss_conflict.png pattern", got)
	})
}

// --- ClipboardFilename tests ---

func TestClipboardFilename(t *testing.T) {
	got := ClipboardFilename()

	// Verify format: YYYYMMDD-HHmmss_clipboard.png
	pattern := regexp.MustCompile(`^\d{8}-\d{6}_clipboard\.png$`)
	assert.True(t, pattern.MatchString(got),
		"clipboard filename %q should match YYYYMMDD-HHmmss_clipboard.png pattern", got)

	assert.True(t, strings.HasSuffix(got, CollisionSeparator+ClipboardFilenamePrefix+ClipboardFilenameExt))
}

// --- ListDirectory tests ---

func TestListDirectory(t *testing.T) {
	root := t.TempDir()

	// Create test structure: 2 dirs, 3 files with different sizes and times.
	require.NoError(t, os.Mkdir(filepath.Join(root, "beta_dir"), DirPermissions))
	require.NoError(t, os.Mkdir(filepath.Join(root, "alpha_dir"), DirPermissions))
	require.NoError(t, os.WriteFile(filepath.Join(root, "small.txt"), []byte("hi"), FilePermissions))
	require.NoError(t, os.WriteFile(filepath.Join(root, "big.txt"), bytes.Repeat([]byte("x"), 2048), FilePermissions))
	require.NoError(t, os.WriteFile(filepath.Join(root, "medium.txt"), bytes.Repeat([]byte("m"), 512), FilePermissions))

	t.Run("default sort dirs first then name asc", func(t *testing.T) {
		entries, err := ListDirectory(root, "", SortByName, SortOrderAsc)
		require.NoError(t, err)
		require.Len(t, entries, 5)

		// Directories first, sorted by name.
		assert.True(t, entries[0].IsDir)
		assert.Equal(t, "alpha_dir", entries[0].Name)
		assert.True(t, entries[1].IsDir)
		assert.Equal(t, "beta_dir", entries[1].Name)

		// Files after, sorted by name.
		assert.False(t, entries[2].IsDir)
		assert.Equal(t, "big.txt", entries[2].Name)
		assert.False(t, entries[3].IsDir)
		assert.Equal(t, "medium.txt", entries[3].Name)
		assert.False(t, entries[4].IsDir)
		assert.Equal(t, "small.txt", entries[4].Name)
	})

	t.Run("sort by name desc", func(t *testing.T) {
		entries, err := ListDirectory(root, "", SortByName, SortOrderDesc)
		require.NoError(t, err)
		require.Len(t, entries, 5)

		// Dirs still first, but in reverse name order.
		assert.Equal(t, "beta_dir", entries[0].Name)
		assert.Equal(t, "alpha_dir", entries[1].Name)

		// Files in reverse name order.
		assert.Equal(t, "small.txt", entries[2].Name)
		assert.Equal(t, "medium.txt", entries[3].Name)
		assert.Equal(t, "big.txt", entries[4].Name)
	})

	t.Run("sort by size", func(t *testing.T) {
		entries, err := ListDirectory(root, "", SortBySize, SortOrderAsc)
		require.NoError(t, err)

		// Find file entries only.
		var files []FileEntry
		for _, e := range entries {
			if !e.IsDir {
				files = append(files, e)
			}
		}
		require.Len(t, files, 3)
		assert.Equal(t, "small.txt", files[0].Name)
		assert.Equal(t, "medium.txt", files[1].Name)
		assert.Equal(t, "big.txt", files[2].Name)
	})

	t.Run("sort by date with controlled times", func(t *testing.T) {
		dateRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dateRoot, "old.txt"), []byte("old"), FilePermissions))
		require.NoError(t, os.WriteFile(filepath.Join(dateRoot, "new.txt"), []byte("new"), FilePermissions))
		require.NoError(t, os.WriteFile(filepath.Join(dateRoot, "mid.txt"), []byte("mid"), FilePermissions))

		// Set explicit modification times.
		oldTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		midTime := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		newTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, os.Chtimes(filepath.Join(dateRoot, "old.txt"), oldTime, oldTime))
		require.NoError(t, os.Chtimes(filepath.Join(dateRoot, "mid.txt"), midTime, midTime))
		require.NoError(t, os.Chtimes(filepath.Join(dateRoot, "new.txt"), newTime, newTime))

		entries, err := ListDirectory(dateRoot, "", SortByDate, SortOrderAsc)
		require.NoError(t, err)
		require.Len(t, entries, 3)
		assert.Equal(t, "old.txt", entries[0].Name)
		assert.Equal(t, "mid.txt", entries[1].Name)
		assert.Equal(t, "new.txt", entries[2].Name)

		// Verify desc order too.
		entriesDesc, err := ListDirectory(dateRoot, "", SortByDate, SortOrderDesc)
		require.NoError(t, err)
		assert.Equal(t, "new.txt", entriesDesc[0].Name)
		assert.Equal(t, "old.txt", entriesDesc[2].Name)
	})

	t.Run("empty directory", func(t *testing.T) {
		entries, err := ListDirectory(root, "alpha_dir", SortByName, SortOrderAsc)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("path jail enforced on listing", func(t *testing.T) {
		_, err := ListDirectory(root, "../etc", SortByName, SortOrderAsc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgPathTraversal)
	})

	t.Run("formatted size populated", func(t *testing.T) {
		entries, err := ListDirectory(root, "", SortByName, SortOrderAsc)
		require.NoError(t, err)

		for _, e := range entries {
			if !e.IsDir && e.Size > 0 {
				assert.NotEmpty(t, e.FormattedSize)
			}
		}
	})

	t.Run("not a directory error", func(t *testing.T) {
		_, err := ListDirectory(root, "small.txt", SortByName, SortOrderAsc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgNotDirectory)
	})
}

// --- SafeWriteFile tests ---

func TestSafeWriteFile(t *testing.T) {
	logger := testLogger()

	t.Run("successful write", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("hello world"))

		finalName, err := SafeWriteFile(root, "", "test.txt", data, DefaultMaxUploadBytes, nil, false, logger)
		require.NoError(t, err)
		assert.Equal(t, "test.txt", finalName)

		content, err := os.ReadFile(filepath.Join(root, "test.txt"))
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(content))
	})

	t.Run("file permissions set correctly", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("perm check"))

		finalName, err := SafeWriteFile(root, "", "perms.txt", data, DefaultMaxUploadBytes, nil, false, logger)
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(root, finalName))
		require.NoError(t, err)
		assert.Equal(t, FilePermissions, info.Mode().Perm())
	})

	t.Run("extension rejected no file on disk", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("data"))

		_, err := SafeWriteFile(root, "", "evil.exe", data, DefaultMaxUploadBytes, []string{".png", ".jpg"}, false, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgExtNotAllowed)
		// Verify error does NOT contain the extension (no info leakage).
		assert.NotContains(t, err.Error(), ".exe")

		// Verify no file was created.
		entries, _ := os.ReadDir(root)
		assert.Empty(t, entries)
	})

	t.Run("readonly mode rejects write", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("data"))

		_, err := SafeWriteFile(root, "", "test.txt", data, DefaultMaxUploadBytes, nil, true, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgReadonlyMode)

		entries, _ := os.ReadDir(root)
		assert.Empty(t, entries)
	})

	t.Run("collision resolution", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "existing.txt"), []byte("old"), FilePermissions))

		data := bytes.NewReader([]byte("new content"))
		finalName, err := SafeWriteFile(root, "", "existing.txt", data, DefaultMaxUploadBytes, nil, false, logger)
		require.NoError(t, err)

		assert.NotEqual(t, "existing.txt", finalName)
		assert.True(t, strings.HasSuffix(finalName, CollisionSeparator+"existing.txt"))

		// Original file untouched.
		original, err := os.ReadFile(filepath.Join(root, "existing.txt"))
		require.NoError(t, err)
		assert.Equal(t, "old", string(original))

		// New file has correct content.
		newContent, err := os.ReadFile(filepath.Join(root, finalName))
		require.NoError(t, err)
		assert.Equal(t, "new content", string(newContent))
	})

	t.Run("filename sanitized", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("safe content"))

		finalName, err := SafeWriteFile(root, "", "my file (1).txt", data, DefaultMaxUploadBytes, nil, false, logger)
		require.NoError(t, err)
		assert.Equal(t, "my_file__1_.txt", finalName)

		_, err = os.Stat(filepath.Join(root, finalName))
		require.NoError(t, err)
	})

	t.Run("max bytes exceeded", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader(bytes.Repeat([]byte("x"), 2048))

		_, err := SafeWriteFile(root, "", "big.txt", data, 1024, nil, false, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgFileTooLarge)

		// Verify no file remains.
		entries, _ := os.ReadDir(root)
		assert.Empty(t, entries)
	})

	t.Run("max bytes exact boundary passes", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader(bytes.Repeat([]byte("x"), 1024))

		finalName, err := SafeWriteFile(root, "", "exact.txt", data, 1024, nil, false, logger)
		require.NoError(t, err)
		assert.Equal(t, "exact.txt", finalName)

		content, err := os.ReadFile(filepath.Join(root, "exact.txt"))
		require.NoError(t, err)
		assert.Len(t, content, 1024)
	})

	t.Run("temp files cleaned on failure", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader(bytes.Repeat([]byte("x"), 100))

		// Reject by extension — nothing should remain.
		_, err := SafeWriteFile(root, "", "file.exe", data, DefaultMaxUploadBytes, []string{".png"}, false, logger)
		require.Error(t, err)

		entries, _ := os.ReadDir(root)
		assert.Empty(t, entries)
	})

	t.Run("write to subdirectory", func(t *testing.T) {
		root := t.TempDir()
		subdir := filepath.Join(root, "uploads")
		require.NoError(t, os.Mkdir(subdir, DirPermissions))

		data := bytes.NewReader([]byte("subdir content"))
		finalName, err := SafeWriteFile(root, "uploads", "doc.pdf", data, DefaultMaxUploadBytes, nil, false, logger)
		require.NoError(t, err)
		assert.Equal(t, "doc.pdf", finalName)

		content, err := os.ReadFile(filepath.Join(subdir, "doc.pdf"))
		require.NoError(t, err)
		assert.Equal(t, "subdir content", string(content))
	})

	t.Run("path traversal in directory rejected", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("escape"))

		_, err := SafeWriteFile(root, "../etc", "passwd", data, DefaultMaxUploadBytes, nil, false, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgPathTraversal)
	})
}

// --- CreateDirectory tests ---

func TestCreateDirectory(t *testing.T) {
	logger := testLogger()

	t.Run("successful creation", func(t *testing.T) {
		root := t.TempDir()
		name, err := CreateDirectory(root, "", "newdir", false, logger)
		require.NoError(t, err)
		assert.Equal(t, "newdir", name)

		info, err := os.Stat(filepath.Join(root, "newdir"))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("readonly mode rejects", func(t *testing.T) {
		root := t.TempDir()
		_, err := CreateDirectory(root, "", "newdir", true, logger)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrReadonlyMode))

		_, err = os.Stat(filepath.Join(root, "newdir"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("name sanitized and returned", func(t *testing.T) {
		root := t.TempDir()
		name, err := CreateDirectory(root, "", "my dir (test)", false, logger)
		require.NoError(t, err)
		assert.Equal(t, "my_dir__test_", name)

		info, err := os.Stat(filepath.Join(root, "my_dir__test_"))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("parent path jailed", func(t *testing.T) {
		root := t.TempDir()
		_, err := CreateDirectory(root, "../etc", "escape", false, logger)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrPathTraversal))
	})

	t.Run("already exists returns error", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "existing"), DirPermissions))

		_, err := CreateDirectory(root, "", "existing", false, logger)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrCreateDir))
	})

	t.Run("nested creation in subdirectory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "parent"), DirPermissions))

		name, err := CreateDirectory(root, "parent", "child", false, logger)
		require.NoError(t, err)
		assert.Equal(t, "child", name)

		info, err := os.Stat(filepath.Join(root, "parent", "child"))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

// --- DC-09 resolveWithAncestorWalk tests ---

func TestSafePath_NonExistentNestedPath(t *testing.T) {
	root := t.TempDir()
	// Create a real parent but request a non-existent child — exercises ancestor walk.
	require.NoError(t, os.Mkdir(filepath.Join(root, "exists"), DirPermissions))

	// Non-existent file under existing parent — ancestor walk resolves "exists" then appends "ghost.txt".
	resolved, err := SafePath(root, "exists/ghost.txt")
	require.NoError(t, err)
	assert.Contains(t, resolved, "exists")
	assert.Contains(t, resolved, "ghost.txt")
}

func TestSafePath_DeeplyNonExistentPath(t *testing.T) {
	root := t.TempDir()
	// Multiple non-existent levels — ancestor walk goes all the way to root.
	resolved, err := SafePath(root, "a/b/c/d/file.txt")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(resolved, root))
	assert.Contains(t, resolved, "a")
}

// --- DC-09 SafePath max length test ---

func TestSafePath_MaxPathLength(t *testing.T) {
	root := t.TempDir()

	longPath := strings.Repeat("a", MaxPathLength+1)
	_, err := SafePath(root, longPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgPathTooLong)

	// Exactly at limit should not trigger the length guard (may fail for other reasons).
	atLimit := strings.Repeat("a", MaxPathLength)
	_, err = SafePath(root, atLimit)
	// Should NOT contain path too long error — may fail with not-found or traversal, but not length.
	if err != nil {
		assert.NotContains(t, err.Error(), ErrMsgPathTooLong)
	}
}

// --- DC-09 concurrent write test ---

func TestSafeWriteFile_ConcurrentWrites(t *testing.T) {
	root := t.TempDir()
	logger := testLogger()

	// Each goroutine writes a uniquely-named file. This tests thread safety
	// of the SafeWriteFile pipeline (temp files, rename) under concurrency.
	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			data := bytes.NewReader([]byte("content from goroutine"))
			filename := "concurrent_" + strings.Repeat("x", n+1) + ".txt"
			name, err := SafeWriteFile(root, "", filename, data, DefaultMaxUploadBytes, nil, false, logger)
			results[n] = name
			errs[n] = err
		}(i)
	}

	wg.Wait()

	// All writes must succeed.
	for i, err := range errs {
		require.NoError(t, err, "goroutine %d should not error", i)
	}

	// All filenames must be unique.
	nameSet := make(map[string]bool, goroutines)
	for i, name := range results {
		assert.NotEmpty(t, name, "goroutine %d should return a filename", i)
		assert.False(t, nameSet[name], "goroutine %d filename %q should be unique", i, name)
		nameSet[name] = true
	}

	// Exactly N files on disk, no temp files left behind.
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Len(t, entries, goroutines, "should have exactly %d files on disk", goroutines)
}

// TestSafeWriteFile_CollisionResolution_Sequential tests that two sequential writes
// to the same filename correctly resolve collisions.
func TestSafeWriteFile_CollisionResolution_Sequential(t *testing.T) {
	root := t.TempDir()
	logger := testLogger()

	// First write — no collision.
	data1 := bytes.NewReader([]byte("first"))
	name1, err := SafeWriteFile(root, "", "dup.txt", data1, DefaultMaxUploadBytes, nil, false, logger)
	require.NoError(t, err)
	assert.Equal(t, "dup.txt", name1)

	// Second write — collision, should get timestamp prefix.
	data2 := bytes.NewReader([]byte("second"))
	name2, err := SafeWriteFile(root, "", "dup.txt", data2, DefaultMaxUploadBytes, nil, false, logger)
	require.NoError(t, err)
	assert.NotEqual(t, name1, name2)
	assert.Contains(t, name2, "dup.txt")

	// Both files should exist with correct content.
	c1, err := os.ReadFile(filepath.Join(root, name1))
	require.NoError(t, err)
	assert.Equal(t, "first", string(c1))

	c2, err := os.ReadFile(filepath.Join(root, name2))
	require.NoError(t, err)
	assert.Equal(t, "second", string(c2))
}
