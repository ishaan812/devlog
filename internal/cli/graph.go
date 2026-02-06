package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/db"
)

var (
	graphType     string
	graphOutput   string
	graphMaxNodes int
	graphCodebase string
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Visualize codebase structure as a graph",
	Long: `Generate visual representations of your codebase structure.

Supports multiple graph types and output formats including Mermaid diagrams
that can be rendered in GitHub, GitLab, or any Mermaid-compatible viewer.

Graph Types:
  structure  - Folder and file hierarchy (default)
  commits    - Developer commit relationships
  files      - File change frequency heatmap
  collab     - Developer collaboration network

Examples:
  devlog graph                          # Show structure in terminal
  devlog graph --type structure         # Folder structure
  devlog graph --type commits           # Commit activity graph
  devlog graph --output graph.md        # Export as Mermaid markdown
  devlog graph --type collab            # Collaboration network`,
	RunE: runGraph,
}

func init() {
	rootCmd.AddCommand(graphCmd)

	graphCmd.Flags().StringVarP(&graphType, "type", "t", "structure", "Graph type (structure, commits, files, collab)")
	graphCmd.Flags().StringVarP(&graphOutput, "output", "o", "", "Output file (default: terminal)")
	graphCmd.Flags().IntVar(&graphMaxNodes, "max-nodes", 50, "Maximum nodes to display")
	graphCmd.Flags().StringVar(&graphCodebase, "codebase", "", "Codebase path (default: current directory)")
}

func runGraph(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	titleColor := color.New(color.FgHiCyan, color.Bold)
	infoColor := color.New(color.FgHiWhite)
	dimColor := color.New(color.FgHiBlack)
	accentColor := color.New(color.FgHiMagenta)

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	var mermaid string

	switch graphType {
	case "structure":
		mermaid, err = generateStructureGraph(ctx, dbRepo, graphCodebase, graphMaxNodes)
	case "commits":
		mermaid, err = generateCommitsGraph(ctx, dbRepo, graphMaxNodes)
	case "files":
		mermaid, err = generateFilesGraph(ctx, dbRepo, graphMaxNodes)
	case "collab":
		mermaid, err = generateCollabGraph(ctx, dbRepo, graphMaxNodes)
	default:
		return fmt.Errorf("unknown graph type: %s", graphType)
	}
	_ = infoColor
	_ = accentColor

	if err != nil {
		return err
	}

	// Output
	if graphOutput != "" {
		content := fmt.Sprintf("# DevLog Graph - %s\n\n```mermaid\n%s\n```\n", graphType, mermaid)
		if err := os.WriteFile(graphOutput, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("Graph saved to %s\n", graphOutput)
		return nil
	}

	// Terminal display
	fmt.Println()
	titleColor.Printf("  DevLog Graph: %s\n", strings.Title(graphType))
	dimColor.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	// Print mermaid with syntax highlighting
	lines := strings.Split(mermaid, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "graph") || strings.HasPrefix(line, "flowchart") {
			accentColor.Printf("  %s\n", line)
		} else if strings.Contains(line, "-->") || strings.Contains(line, "---") {
			infoColor.Printf("  %s\n", line)
		} else if strings.TrimSpace(line) != "" {
			dimColor.Printf("  %s\n", line)
		}
	}

	fmt.Println()
	dimColor.Println("  Copy the above Mermaid code to visualize in:")
	dimColor.Println("  - GitHub/GitLab markdown files")
	dimColor.Println("  - https://mermaid.live")
	dimColor.Println("  - VS Code with Mermaid extension")
	fmt.Println()

	return nil
}

func generateStructureGraph(ctx context.Context, dbRepo *db.SQLRepository, codebasePath string, maxNodes int) (string, error) {
	if codebasePath == "" {
		var absErr error
		codebasePath, absErr = filepath.Abs(".")
		if absErr != nil {
			return "", fmt.Errorf("failed to resolve current directory: %w", absErr)
		}
	}

	codebase, err := dbRepo.GetCodebaseByPath(ctx, codebasePath)
	if err != nil {
		return "", err
	}
	if codebase == nil {
		return "", fmt.Errorf("codebase not indexed. Run 'devlog ingest' first")
	}

	folders, err := dbRepo.GetFoldersByCodebase(ctx, codebase.ID)
	if err != nil {
		return "", err
	}

	files, err := dbRepo.GetFilesByCodebase(ctx, codebase.ID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("flowchart TD\n")

	// Add root
	sb.WriteString(fmt.Sprintf("    root[\"📁 %s\"]\n", codebase.Name))

	// Track nodes
	nodeCount := 1
	folderNodes := make(map[string]string)
	folderNodes["."] = "root"

	// Add folders (limit depth to keep it readable)
	for _, f := range folders {
		if nodeCount >= maxNodes {
			break
		}
		if f.Depth > 2 {
			continue
		}

		nodeID := fmt.Sprintf("f%d", nodeCount)
		folderNodes[f.Path] = nodeID

		icon := "📁"
		if f.Purpose != "" {
			switch {
			case strings.Contains(strings.ToLower(f.Purpose), "api"):
				icon = "🔌"
			case strings.Contains(strings.ToLower(f.Purpose), "test"):
				icon = "🧪"
			case strings.Contains(strings.ToLower(f.Purpose), "config"):
				icon = "⚙️"
			case strings.Contains(strings.ToLower(f.Purpose), "ui"), strings.Contains(strings.ToLower(f.Purpose), "component"):
				icon = "🎨"
			case strings.Contains(strings.ToLower(f.Purpose), "model"), strings.Contains(strings.ToLower(f.Purpose), "database"):
				icon = "💾"
			}
		}

		label := f.Name
		if f.Summary != "" && len(f.Summary) < 30 {
			label = fmt.Sprintf("%s\\n%s", f.Name, f.Summary)
		}

		sb.WriteString(fmt.Sprintf("    %s[\"%s %s\"]\n", nodeID, icon, label))

		// Connect to parent
		parentNode := "root"
		if f.ParentPath != "" {
			if pn, ok := folderNodes[f.ParentPath]; ok {
				parentNode = pn
			}
		}
		sb.WriteString(fmt.Sprintf("    %s --> %s\n", parentNode, nodeID))
		nodeCount++
	}

	// Add some key files
	filesByFolder := make(map[string][]db.FileIndex)
	for _, f := range files {
		folderPath := filepath.Dir(f.Path)
		if folderPath == "." {
			folderPath = "."
		}
		filesByFolder[folderPath] = append(filesByFolder[folderPath], f)
	}

	for folderPath, folderFiles := range filesByFolder {
		if nodeCount >= maxNodes {
			break
		}

		parentNode, ok := folderNodes[folderPath]
		if !ok {
			continue
		}

		// Only show important files (with summaries or main files)
		for _, f := range folderFiles {
			if nodeCount >= maxNodes {
				break
			}

			isImportant := f.Summary != "" ||
				f.Name == "main.go" || f.Name == "index.js" || f.Name == "index.ts" ||
				f.Name == "app.py" || f.Name == "package.json" || f.Name == "go.mod"

			if !isImportant && len(folderFiles) > 5 {
				continue
			}

			nodeID := fmt.Sprintf("file%d", nodeCount)
			icon := getFileIcon(f.Extension, f.Language)

			sb.WriteString(fmt.Sprintf("    %s([\"%s %s\"])\n", nodeID, icon, f.Name))
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", parentNode, nodeID))
			nodeCount++
		}
	}

	// Styling
	sb.WriteString("\n    %% Styling\n")
	sb.WriteString("    classDef folder fill:#e1f5fe,stroke:#01579b\n")
	sb.WriteString("    classDef file fill:#fff3e0,stroke:#e65100\n")

	return sb.String(), nil
}

func generateCommitsGraph(ctx context.Context, dbRepo *db.SQLRepository, maxNodes int) (string, error) {
	type AuthorStats struct {
		AuthorEmail string
		CommitCount int64
	}

	results, err := dbRepo.ExecuteQuery(ctx, fmt.Sprintf(`
		SELECT author_email, COUNT(*) as commit_count
		FROM commits GROUP BY author_email ORDER BY commit_count DESC LIMIT %d
	`, maxNodes))
	if err != nil {
		return "", err
	}

	var authors []AuthorStats
	for _, row := range results {
		a := AuthorStats{}
		if v, ok := row["author_email"].(string); ok {
			a.AuthorEmail = v
		}
		if v, ok := row["commit_count"].(int64); ok {
			a.CommitCount = v
		}
		authors = append(authors, a)
	}

	var sb strings.Builder
	sb.WriteString("flowchart LR\n")

	// Add author nodes
	for i, a := range authors {
		nodeID := fmt.Sprintf("dev%d", i)
		name := strings.Split(a.AuthorEmail, "@")[0]
		sb.WriteString(fmt.Sprintf("    %s((\"👤 %s\\n%d commits\"))\n", nodeID, name, a.CommitCount))
	}

	// Add repo node
	sb.WriteString("    repo[(\"📦 Repository\")]\n")

	// Connect authors to repo with weighted edges
	for i, a := range authors {
		nodeID := fmt.Sprintf("dev%d", i)
		weight := "---"
		if a.CommitCount > 50 {
			weight = "==>"
		} else if a.CommitCount > 10 {
			weight = "-->"
		}
		sb.WriteString(fmt.Sprintf("    %s %s|%d| repo\n", nodeID, weight, a.CommitCount))
	}

	return sb.String(), nil
}

func generateFilesGraph(ctx context.Context, dbRepo *db.SQLRepository, maxNodes int) (string, error) {
	type FileStats struct {
		FilePath    string
		ChangeCount int64
	}

	results, err := dbRepo.ExecuteQuery(ctx, fmt.Sprintf(`
		SELECT file_path, COUNT(*) as change_count
		FROM file_changes GROUP BY file_path ORDER BY change_count DESC LIMIT %d
	`, maxNodes))
	if err != nil {
		return "", err
	}

	var fileStats []FileStats
	for _, row := range results {
		fs := FileStats{}
		if v, ok := row["file_path"].(string); ok {
			fs.FilePath = v
		}
		if v, ok := row["change_count"].(int64); ok {
			fs.ChangeCount = v
		}
		fileStats = append(fileStats, fs)
	}

	var sb strings.Builder
	sb.WriteString("flowchart TD\n")
	sb.WriteString("    subgraph hotspots[\"🔥 Most Changed Files\"]\n")

	for i, fs := range fileStats {
		nodeID := fmt.Sprintf("f%d", i)
		name := filepath.Base(fs.FilePath)
		ext := filepath.Ext(name)
		icon := getFileIcon(ext, "")

		// Color based on change frequency
		style := ""
		if fs.ChangeCount > 20 {
			style = ":::hot"
		} else if fs.ChangeCount > 10 {
			style = ":::warm"
		}

		sb.WriteString(fmt.Sprintf("        %s[\"%s %s\\n%d changes\"]%s\n", nodeID, icon, name, fs.ChangeCount, style))
	}

	sb.WriteString("    end\n")
	sb.WriteString("\n    classDef hot fill:#ff5252,color:#fff\n")
	sb.WriteString("    classDef warm fill:#ffab40\n")

	return sb.String(), nil
}

func generateCollabGraph(ctx context.Context, dbRepo *db.SQLRepository, maxNodes int) (string, error) {
	type CollabEdge struct {
		Dev1        string
		Dev2        string
		SharedFiles int64
	}

	results, err := dbRepo.ExecuteQuery(ctx, fmt.Sprintf(`
		WITH file_authors AS (
			SELECT DISTINCT fc.file_path, c.author_email
			FROM file_changes fc JOIN commits c ON fc.commit_id = c.id
		)
		SELECT a1.author_email as dev1, a2.author_email as dev2, COUNT(DISTINCT a1.file_path) as shared_files
		FROM file_authors a1
		JOIN file_authors a2 ON a1.file_path = a2.file_path AND a1.author_email < a2.author_email
		GROUP BY a1.author_email, a2.author_email
		HAVING COUNT(DISTINCT a1.file_path) > 1
		ORDER BY shared_files DESC LIMIT %d
	`, maxNodes))
	if err != nil {
		return "", err
	}

	var edges []CollabEdge
	for _, row := range results {
		e := CollabEdge{}
		if v, ok := row["dev1"].(string); ok {
			e.Dev1 = v
		}
		if v, ok := row["dev2"].(string); ok {
			e.Dev2 = v
		}
		if v, ok := row["shared_files"].(int64); ok {
			e.SharedFiles = v
		}
		edges = append(edges, e)
	}

	var sb strings.Builder
	sb.WriteString("flowchart LR\n")
	sb.WriteString("    subgraph collab[\"👥 Collaboration Network\"]\n")

	developers := make(map[string]string)
	nodeCount := 0

	for _, e := range edges {
		// Add developer nodes
		for _, dev := range []string{e.Dev1, e.Dev2} {
			if _, exists := developers[dev]; !exists {
				nodeID := fmt.Sprintf("d%d", nodeCount)
				developers[dev] = nodeID
				name := strings.Split(dev, "@")[0]
				sb.WriteString(fmt.Sprintf("        %s((\"👤 %s\"))\n", nodeID, name))
				nodeCount++
			}
		}
	}

	// Add edges
	for _, e := range edges {
		n1 := developers[e.Dev1]
		n2 := developers[e.Dev2]
		weight := "---"
		if e.SharedFiles > 10 {
			weight = "==="
		} else if e.SharedFiles > 5 {
			weight = "---"
		}
		sb.WriteString(fmt.Sprintf("        %s %s|%d files| %s\n", n1, weight, e.SharedFiles, n2))
	}

	sb.WriteString("    end\n")

	return sb.String(), nil
}

func getFileIcon(ext, language string) string {
	icons := map[string]string{
		".go":    "🔷",
		".py":    "🐍",
		".js":    "📜",
		".ts":    "📘",
		".jsx":   "⚛️",
		".tsx":   "⚛️",
		".java":  "☕",
		".rs":    "🦀",
		".rb":    "💎",
		".php":   "🐘",
		".c":     "🔧",
		".cpp":   "🔧",
		".cs":    "🟣",
		".swift": "🍎",
		".kt":    "🟠",
		".sql":   "💾",
		".html":  "🌐",
		".css":   "🎨",
		".json":  "📋",
		".yaml":  "📋",
		".yml":   "📋",
		".md":    "📝",
		".sh":    "🖥️",
	}

	if icon, ok := icons[ext]; ok {
		return icon
	}
	return "📄"
}
