package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
)

const cronMarkerPrefix = "# devlog-cron:"

var (
	cronHour       int
	cronMinute     int
	cronIngestDays int
	cronLogPath    string
	cronDryRun     bool
	cronYes        bool
	cronRemoveAll  bool
	cronRemoveYes  bool
)

var cronCmd = &cobra.Command{
	Use:   "cron [path]",
	Short: "Set up a daily cron job for ingestion",
	Long: `Set up a daily cron job that runs DevLog ingestion and auto-generates a worklog.

The cron job runs:
  devlog ingest --all-branches --days <N> --auto-worklog

Examples:
  devlog cron
  devlog cron ~/projects/myapp --hour 7 --minute 30
  devlog cron --days 1`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCronSetup,
}

var cronRemoveCmd = &cobra.Command{
	Use:   "remove [path]",
	Short: "Remove a configured devlog cron job",
	Long: `Remove one or more DevLog cron jobs.

Examples:
  devlog cron remove
  devlog cron remove ~/projects/myapp
  devlog cron remove --all`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCronRemove,
}

func init() {
	rootCmd.AddCommand(cronCmd)
	cronCmd.AddCommand(cronRemoveCmd)

	cronCmd.Flags().IntVar(&cronHour, "hour", 9, "Hour of day in 24h format (0-23)")
	cronCmd.Flags().IntVar(&cronMinute, "minute", 0, "Minute of hour (0-59)")
	cronCmd.Flags().IntVar(&cronIngestDays, "days", 1, "Ingest window in days for each scheduled run")
	cronCmd.Flags().StringVar(&cronLogPath, "log", "", "Optional log file path for cron output")
	cronCmd.Flags().BoolVar(&cronDryRun, "dry-run", false, "Print the cron entry without installing it")
	cronCmd.Flags().BoolVarP(&cronYes, "yes", "y", false, "Skip confirmation prompt and install immediately")

	cronRemoveCmd.Flags().BoolVar(&cronRemoveAll, "all", false, "Remove all DevLog-managed cron jobs")
	cronRemoveCmd.Flags().BoolVarP(&cronRemoveYes, "yes", "y", false, "Skip confirmation prompt and remove immediately")
}

func runCronSetup(cmd *cobra.Command, args []string) error {
	if cronHour < 0 || cronHour > 23 {
		return fmt.Errorf("invalid --hour %d (must be 0-23)", cronHour)
	}
	if cronMinute < 0 || cronMinute > 59 {
		return fmt.Errorf("invalid --minute %d (must be 0-59)", cronMinute)
	}
	if cronIngestDays <= 0 {
		return fmt.Errorf("invalid --days %d (must be > 0)", cronIngestDays)
	}

	absPath, profileName, err := resolveCronTargetAndProfile(args)
	if err != nil {
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
		execPath = resolved
	}

	logPath := cronLogPath
	if strings.TrimSpace(logPath) == "" {
		logDir := filepath.Join(config.GetDevlogDir(), "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("create log directory: %w", err)
		}
		logPath = filepath.Join(logDir, fmt.Sprintf("cron-%s-%s.log", sanitizeToken(profileName), sanitizeToken(filepath.Base(absPath))))
	}
	logPath, err = filepath.Abs(logPath)
	if err != nil {
		return fmt.Errorf("resolve log path: %w", err)
	}

	cronEntry := buildCronEntry(absPath, execPath, profileName, logPath)
	marker := cronMarker(profileName, absPath)

	if cronDryRun {
		fmt.Println(cronEntry)
		return nil
	}

	if !cronYes {
		infoColor := color.New(color.FgCyan)
		dimColor := color.New(color.FgHiBlack)
		promptColor := color.New(color.FgYellow)

		infoColor.Printf("DevLog will set up a daily cron job on this computer at %02d:%02d.\n", cronHour, cronMinute)
		dimColor.Printf("Repo: %s\n", absPath)
		dimColor.Printf("Profile: %s\n", profileName)
		dimColor.Println("To use a different schedule, run:")
		dimColor.Printf("  devlog cron %s --hour 7 --minute 30\n", shellQuote(absPath))
		dimColor.Println("To remove later, run:")
		dimColor.Printf("  devlog cron remove %s\n", shellQuote(absPath))
		fmt.Println()
		promptColor.Print("Proceed? [y/N]: ")
		if !confirmYesDefaultNo() {
			dimColor.Println("Canceled. No cron job was installed.")
			return nil
		}
	}

	existing, err := readCrontab()
	if err != nil {
		return err
	}

	lines := splitLinesPreserveNonEmpty(existing)
	filtered := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		if strings.Contains(line, marker) {
			continue
		}
		filtered = append(filtered, line)
	}
	filtered = append(filtered, cronEntry)
	newCrontab := strings.Join(filtered, "\n") + "\n"

	if err := writeCrontab(newCrontab); err != nil {
		return err
	}

	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	successColor.Println("Cron job installed.")
	dimColor.Printf("Schedule: daily at %02d:%02d\n", cronHour, cronMinute)
	dimColor.Printf("Repo: %s\n", absPath)
	dimColor.Printf("Profile: %s\n", profileName)
	dimColor.Printf("Log: %s\n", logPath)
	return nil
}

func runCronRemove(cmd *cobra.Command, args []string) error {
	if cronRemoveAll && len(args) > 0 {
		return fmt.Errorf("cannot use [path] with --all")
	}

	absPath := ""
	profileName := ""
	if !cronRemoveAll {
		var err error
		absPath, profileName, err = resolveCronTargetAndProfile(args)
		if err != nil {
			return err
		}
	}

	existing, err := readCrontab()
	if err != nil {
		return err
	}

	lines := splitLinesPreserveNonEmpty(existing)
	if len(lines) == 0 {
		color.New(color.FgHiBlack).Println("No cron jobs found.")
		return nil
	}

	filtered := make([]string, 0, len(lines))
	removedCount := 0
	marker := ""
	if !cronRemoveAll {
		marker = cronMarker(profileName, absPath)
	}
	for _, line := range lines {
		if cronRemoveAll {
			if strings.Contains(line, cronMarkerPrefix) {
				removedCount++
				continue
			}
		} else if strings.Contains(line, marker) {
			removedCount++
			continue
		}
		filtered = append(filtered, line)
	}

	dimColor := color.New(color.FgHiBlack)
	promptColor := color.New(color.FgYellow)
	successColor := color.New(color.FgHiGreen)
	if removedCount == 0 {
		if cronRemoveAll {
			dimColor.Println("No DevLog cron jobs found to remove.")
		} else {
			dimColor.Println("No matching DevLog cron job found for this repo/profile.")
		}
		return nil
	}

	if !cronRemoveYes {
		if cronRemoveAll {
			promptColor.Printf("Remove %d DevLog cron job(s)? [y/N]: ", removedCount)
		} else {
			promptColor.Printf("Remove DevLog cron job for repo '%s'? [y/N]: ", absPath)
		}
		if !confirmYesDefaultNo() {
			dimColor.Println("Canceled. No cron jobs were removed.")
			return nil
		}
	}

	newCrontab := ""
	if len(filtered) > 0 {
		newCrontab = strings.Join(filtered, "\n") + "\n"
	}
	if err := writeCrontab(newCrontab); err != nil {
		return err
	}

	successColor.Printf("Removed %d DevLog cron job(s).\n", removedCount)
	return nil
}

func buildCronEntry(absPath, execPath, profileName, logPath string) string {
	quotedRepo := shellQuote(absPath)
	quotedExec := shellQuote(execPath)
	quotedProfile := shellQuote(profileName)
	quotedLog := shellQuote(logPath)
	marker := cronMarker(profileName, absPath)

	command := fmt.Sprintf("cd %s && %s --profile %s ingest --all-branches --days %d --auto-worklog", quotedRepo, quotedExec, quotedProfile, cronIngestDays)
	return fmt.Sprintf("%d %d * * * %s >> %s 2>&1 %s", cronMinute, cronHour, command, quotedLog, marker)
}

func cronMarker(profileName, absPath string) string {
	identifier := sanitizeToken(profileName) + "_" + sanitizeToken(absPath)
	return cronMarkerPrefix + identifier
}

func readCrontab() (string, error) {
	cmd := exec.Command("crontab", "-l")
	out, err := cmd.Output()
	if err == nil {
		return string(out), nil
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return "", fmt.Errorf("failed to run crontab -l: %w", err)
	}
	stderr := strings.ToLower(strings.TrimSpace(string(exitErr.Stderr)))
	if strings.Contains(stderr, "no crontab") {
		return "", nil
	}
	return "", fmt.Errorf("failed to read crontab: %s", strings.TrimSpace(string(exitErr.Stderr)))
}

func writeCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install crontab: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func sanitizeToken(s string) string {
	if strings.TrimSpace(s) == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "default"
	}
	return out
}

func splitLinesPreserveNonEmpty(content string) []string {
	raw := strings.Split(content, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func resolveCronTargetAndProfile(args []string) (string, string, error) {
	targetPath := "."
	if len(args) > 0 {
		targetPath = args[0]
	}
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return "", "", fmt.Errorf("failed to load config: %w", err)
	}
	profileName := cfg.GetActiveProfileName()
	if profileFlag != "" {
		profileName = profileFlag
	}
	return absPath, profileName, nil
}

func confirmYesDefaultNo() bool {
	var input string
	_, _ = fmt.Scanln(&input)
	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes"
}
