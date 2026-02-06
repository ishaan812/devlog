package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileInfo holds information about a scanned file
type FileInfo struct {
	Path      string
	Name      string
	Extension string
	Language  string
	Size      int64
	Content   string
	Hash      string
}

// FolderInfo holds information about a scanned folder
type FolderInfo struct {
	Path       string
	Name       string
	Depth      int
	ParentPath string
	Files      []FileInfo
	SubFolders []string
	FileCount  int
}

// ScanResult holds the complete scan result
type ScanResult struct {
	RootPath string
	Name     string
	Folders  map[string]*FolderInfo
	Files    []FileInfo
}

var ignoredDirs = map[string]bool{
	".git":          true,
	"node_modules":  true,
	"vendor":        true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".idea":         true,
	".vscode":       true,
	"dist":          true,
	"build":         true,
	"target":        true,
	".next":         true,
	".nuxt":         true,
	"coverage":      true,
	".pytest_cache": true,
	".mypy_cache":   true,
}

var ignoredExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
	".svg": true, ".webp": true, ".mp3": true, ".mp4": true, ".wav": true,
	".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".rar": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".lock": true, ".sum": true,
}

var languageMap = map[string]string{
	".go":      "Go",
	".py":      "Python",
	".js":      "JavaScript",
	".ts":      "TypeScript",
	".jsx":     "JavaScript (React)",
	".tsx":     "TypeScript (React)",
	".java":    "Java",
	".rs":      "Rust",
	".rb":      "Ruby",
	".php":     "PHP",
	".c":       "C",
	".cpp":     "C++",
	".h":       "C/C++ Header",
	".hpp":     "C++ Header",
	".cs":      "C#",
	".swift":   "Swift",
	".kt":      "Kotlin",
	".scala":   "Scala",
	".r":       "R",
	".sql":     "SQL",
	".sh":      "Shell",
	".bash":    "Bash",
	".zsh":     "Zsh",
	".ps1":     "PowerShell",
	".yaml":    "YAML",
	".yml":     "YAML",
	".json":    "JSON",
	".xml":     "XML",
	".html":    "HTML",
	".css":     "CSS",
	".scss":    "SCSS",
	".sass":    "Sass",
	".less":    "Less",
	".md":      "Markdown",
	".rst":     "reStructuredText",
	".txt":     "Text",
	".toml":    "TOML",
	".ini":     "INI",
	".cfg":     "Config",
	".env":     "Environment",
	".proto":   "Protocol Buffers",
	".graphql": "GraphQL",
	".vue":     "Vue",
	".svelte":  "Svelte",
}

// ScanCodebase scans a directory and returns information about all files and folders
func ScanCodebase(rootPath string, maxFileSize int64) (*ScanResult, error) {
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{
		RootPath: absPath,
		Name:     filepath.Base(absPath),
		Folders:  make(map[string]*FolderInfo),
		Files:    []FileInfo{},
	}

	// Add root folder
	result.Folders["."] = &FolderInfo{
		Path:  ".",
		Name:  result.Name,
		Depth: 0,
	}

	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk error at %s: %w", path, err)
		}

		relPath, err := filepath.Rel(absPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}
		if relPath == "." {
			return nil
		}

		// Check if directory should be ignored
		if d.IsDir() {
			if ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			// Skip hidden directories
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}

			depth := strings.Count(relPath, string(os.PathSeparator)) + 1
			parentPath := filepath.Dir(relPath)
			if parentPath == "." {
				parentPath = ""
			}

			result.Folders[relPath] = &FolderInfo{
				Path:       relPath,
				Name:       d.Name(),
				Depth:      depth,
				ParentPath: parentPath,
			}

			// Add to parent's subfolders
			if parent, ok := result.Folders[parentPath]; ok {
				parent.SubFolders = append(parent.SubFolders, relPath)
			} else if parentPath == "" {
				result.Folders["."].SubFolders = append(result.Folders["."].SubFolders, relPath)
			}

			return nil
		}

		// Process file
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ignoredExtensions[ext] {
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Skip large files
		if maxFileSize > 0 && info.Size() > maxFileSize {
			return nil
		}

		fileInfo := FileInfo{
			Path:      relPath,
			Name:      d.Name(),
			Extension: ext,
			Language:  languageMap[ext],
			Size:      info.Size(),
		}

		// Read file content for small files
		if info.Size() < 100*1024 { // 100KB limit for content
			content, err := os.ReadFile(path)
			if err == nil {
				fileInfo.Content = string(content)
				hash := sha256.Sum256(content)
				fileInfo.Hash = hex.EncodeToString(hash[:])
			}
		}

		result.Files = append(result.Files, fileInfo)

		// Add to folder
		folderPath := filepath.Dir(relPath)
		if folderPath == "." {
			folderPath = "."
		}
		if folder, ok := result.Folders[folderPath]; ok {
			folder.Files = append(folder.Files, fileInfo)
			folder.FileCount++
		}

		return nil
	})

	return result, err
}

// CountLines counts the number of lines in content
func CountLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

// DetectTechStack analyzes files to detect the technology stack
func DetectTechStack(files []FileInfo) map[string]int {
	stack := make(map[string]int)

	for _, f := range files {
		if f.Language != "" {
			stack[f.Language]++
		}

		// Detect by filename
		switch f.Name {
		case "package.json":
			stack["Node.js"]++
		case "go.mod":
			stack["Go"]++
		case "Cargo.toml":
			stack["Rust"]++
		case "requirements.txt", "setup.py", "pyproject.toml":
			stack["Python"]++
		case "Gemfile":
			stack["Ruby"]++
		case "pom.xml", "build.gradle":
			stack["Java"]++
		case "Dockerfile", "docker-compose.yml":
			stack["Docker"]++
		case ".github":
			stack["GitHub Actions"]++
		}
	}

	return stack
}
