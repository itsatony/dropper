package dropper

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		{name: "dash allowed", input: "-file.txt", want: "-file.txt"},
		{name: "special chars replaced", input: "hello world!@#.jpg", want: "hello_world___.jpg"},
		{name: "null byte replaced", input: "\x00evil", want: "_evil"},
		{name: "underscore preserved", input: "my_file_v2.tar.gz", want: "my_file_v2.tar.gz"},
		{name: "all unsafe chars", input: "~`!@#$%^&*()+=[]{}", want: "__________________"},
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
			if got != FilenameFallback {
				assert.True(t, isFilenameSafe(got), "sanitized filename contains unsafe characters: %q", got)
			}
		})
	}
}

// isFilenameSafe checks that a filename contains only allowed characters.
func isFilenameSafe(name string) bool {
	safe := regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)
	return safe.MatchString(name)
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

	rootAbs, err := filepath.Abs(root)
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

	t.Run("empty directory", func(t *testing.T) {
		emptyDir := filepath.Join(root, "alpha_dir")
		entries, err := ListDirectory(root, "alpha_dir", SortByName, SortOrderAsc)
		require.NoError(t, err)
		assert.Empty(t, entries)
		_ = emptyDir // keep linter happy
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
	t.Run("successful write", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("hello world"))

		finalName, err := SafeWriteFile(root, "", "test.txt", data, DefaultMaxUploadBytes, nil, false)
		require.NoError(t, err)
		assert.Equal(t, "test.txt", finalName)

		content, err := os.ReadFile(filepath.Join(root, "test.txt"))
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(content))
	})

	t.Run("extension rejected no file on disk", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("data"))

		_, err := SafeWriteFile(root, "", "evil.exe", data, DefaultMaxUploadBytes, []string{".png", ".jpg"}, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgExtNotAllowed)

		// Verify no file was created.
		entries, _ := os.ReadDir(root)
		assert.Empty(t, entries)
	})

	t.Run("readonly mode rejects write", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("data"))

		_, err := SafeWriteFile(root, "", "test.txt", data, DefaultMaxUploadBytes, nil, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgReadonlyMode)

		entries, _ := os.ReadDir(root)
		assert.Empty(t, entries)
	})

	t.Run("collision resolution", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "existing.txt"), []byte("old"), FilePermissions))

		data := bytes.NewReader([]byte("new content"))
		finalName, err := SafeWriteFile(root, "", "existing.txt", data, DefaultMaxUploadBytes, nil, false)
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

		finalName, err := SafeWriteFile(root, "", "my file (1).txt", data, DefaultMaxUploadBytes, nil, false)
		require.NoError(t, err)
		assert.Equal(t, "my_file__1_.txt", finalName)

		_, err = os.Stat(filepath.Join(root, finalName))
		require.NoError(t, err)
	})

	t.Run("max bytes exceeded", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader(bytes.Repeat([]byte("x"), 2048))

		_, err := SafeWriteFile(root, "", "big.txt", data, 1024, nil, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgFileTooLarge)

		// Verify no file remains.
		entries, _ := os.ReadDir(root)
		assert.Empty(t, entries)
	})

	t.Run("temp files cleaned on failure", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader(bytes.Repeat([]byte("x"), 100))

		// Reject by extension — nothing should remain.
		_, err := SafeWriteFile(root, "", "file.exe", data, DefaultMaxUploadBytes, []string{".png"}, false)
		require.Error(t, err)

		entries, _ := os.ReadDir(root)
		assert.Empty(t, entries)
	})

	t.Run("write to subdirectory", func(t *testing.T) {
		root := t.TempDir()
		subdir := filepath.Join(root, "uploads")
		require.NoError(t, os.Mkdir(subdir, DirPermissions))

		data := bytes.NewReader([]byte("subdir content"))
		finalName, err := SafeWriteFile(root, "uploads", "doc.pdf", data, DefaultMaxUploadBytes, nil, false)
		require.NoError(t, err)
		assert.Equal(t, "doc.pdf", finalName)

		content, err := os.ReadFile(filepath.Join(subdir, "doc.pdf"))
		require.NoError(t, err)
		assert.Equal(t, "subdir content", string(content))
	})

	t.Run("path traversal in directory rejected", func(t *testing.T) {
		root := t.TempDir()
		data := bytes.NewReader([]byte("escape"))

		_, err := SafeWriteFile(root, "../etc", "passwd", data, DefaultMaxUploadBytes, nil, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgPathTraversal)
	})
}

// --- CreateDirectory tests ---

func TestCreateDirectory(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		root := t.TempDir()
		err := CreateDirectory(root, "", "newdir", false)
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(root, "newdir"))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("readonly mode rejects", func(t *testing.T) {
		root := t.TempDir()
		err := CreateDirectory(root, "", "newdir", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgReadonlyMode)

		_, err = os.Stat(filepath.Join(root, "newdir"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("name sanitized", func(t *testing.T) {
		root := t.TempDir()
		err := CreateDirectory(root, "", "my dir (test)", false)
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(root, "my_dir__test_"))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("parent path jailed", func(t *testing.T) {
		root := t.TempDir()
		err := CreateDirectory(root, "../etc", "escape", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgPathTraversal)
	})

	t.Run("already exists returns error", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "existing"), DirPermissions))

		err := CreateDirectory(root, "", "existing", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgCreateDir)
	})

	t.Run("nested creation in subdirectory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "parent"), DirPermissions))

		err := CreateDirectory(root, "parent", "child", false)
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(root, "parent", "child"))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}
