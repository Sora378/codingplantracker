package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Sora378/codingplantracker/internal/api"
	"github.com/Sora378/codingplantracker/internal/config"
	"github.com/Sora378/codingplantracker/internal/models"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "cpq",
	Short: "Track coding plan and AI API usage",
	Long:  `Coplanage monitors coding-plan quotas and can proxy AI API calls to record token usage.`,
	RunE:  runRoot,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default is ~/.config/coplanage/config.json)")
	rootCmd.PersistentFlags().BoolVar(&showInfo, "info", false, "show credential storage info")
	rootCmd.PersistentFlags().BoolVar(&runCLI, "cli", false, "run in CLI mode")
}

var showInfo bool
var runCLI bool

const maxCLIAPIKeyBytes = 8 << 10

func runCLIInteractive() error {
	cfg, err := config.LoadFrom(cfgFile)
	if err != nil {
		cfg = config.DefaultConfigFrom(cfgFile)
	}

	reader := bufio.NewReader(os.Stdin)

	if !cfg.IsLoggedIn() {
		fmt.Println()
		fmt.Println("╔════════════════════════════════════════════════════════════╗")
		fmt.Println("║                        Coplanage                       ║")
		fmt.Println("╚════════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("  You are not logged in.")
		fmt.Println()

		if err := doLogin(cfg, reader); err != nil {
			fmt.Fprintf(os.Stderr, "  Login failed: %v\n", err)
			return nil
		}
		fmt.Println("  Successfully logged in!")
	}

	showMenu(cfg, reader)
	return nil
}

func runRoot(cmd *cobra.Command, args []string) error {
	// If --cli flag was passed, run CLI mode
	if runCLI {
		return runCLIInteractive()
	}

	cfg, err := config.LoadFrom(cfgFile)
	if err != nil {
		cfg = config.DefaultConfigFrom(cfgFile)
	}

	if showInfo {
		fmt.Println()
		cfg.ShowCredentialLocations()
		return nil
	}

	// Default GUI mode (handled in main.go)
	return nil
}

func showMenu(cfg *config.Config, reader *bufio.Reader) {
	for {
		fmt.Println()
		fmt.Println("╔════════════════════════════════════════════════════════════╗")
		fmt.Println("║                        Coplanage                       ║")
		fmt.Println("╚════════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Printf("  Logged in as: %s\n", cfg.Email)
		fmt.Printf("  Region: %s\n\n", cfg.Region)
		fmt.Println("  1. View Status")
		fmt.Println("  2. View History")
		fmt.Println("  3. Refresh")
		fmt.Println("  4. Logout")
		fmt.Println("  q. Quit")
		fmt.Println()

		fmt.Print("  Enter choice: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "1", "":
			showStatus(cfg)
		case "2":
			showHistory(cfg)
		case "3":
			showStatus(cfg)
		case "4":
			if doLogout(cfg) {
				fmt.Println("  Logged out successfully!")
				return
			}
		case "q":
			fmt.Println("  Goodbye!")
			return
		default:
			fmt.Println("  Invalid choice.")
		}
	}
}

func doLogout(cfg *config.Config) bool {
	fmt.Println()
	fmt.Print("  Are you sure you want to logout? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))

	if confirm != "y" && confirm != "yes" {
		return false
	}

	if err := cfg.ClearCredentials(); err != nil {
		fmt.Printf("  Error clearing credentials: %v\n", err)
		return false
	}

	cfg.Email = ""
	cfg.UserID = ""
	cfg.Region = "global"
	if err := cfg.Save(); err != nil {
		fmt.Printf("  Error saving config: %v\n", err)
		return false
	}

	return true
}

func doLogin(cfg *config.Config, reader *bufio.Reader) error {
	fmt.Println()
	fmt.Println("  🔐 Credential Storage Info:")
	cfg.ShowCredentialLocations()
	fmt.Println()

	// Region selection
	fmt.Println("  Select API Region:")
	fmt.Println("  1. Global (https://api.minimax.io)")
	fmt.Println("  2. China (https://api.minimaxi.com)")
	fmt.Println()
	fmt.Print("  Enter choice (1 or 2): ")

	regionChoice, _ := reader.ReadString('\n')
	regionChoice = strings.TrimSpace(regionChoice)

	if regionChoice == "2" {
		cfg.Region = "china"
		fmt.Println("  ✓ Selected: China API")
	} else {
		cfg.Region = "global"
		fmt.Println("  ✓ Selected: Global API")
	}

	fmt.Println()
	fmt.Println("  Get your API key from: https://platform.minimax.io/user-center/basic-information/interface-key")
	fmt.Println()
	fmt.Print("  Enter API Key: ")

	apiKey, err := readLimitedLine(reader, maxCLIAPIKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to read API key: %w", err)
	}
	apiKey = strings.TrimSpace(apiKey)

	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	// Save API key to system keychain (secure storage)
	if err := cfg.SetCredentials(apiKey, "api_key"); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

func readLimitedLine(reader *bufio.Reader, maxBytes int64) (string, error) {
	var b strings.Builder
	for int64(b.Len()) <= maxBytes {
		ch, err := reader.ReadByte()
		if err != nil {
			if b.Len() > 0 {
				return b.String(), nil
			}
			return "", err
		}
		if ch == '\n' {
			return b.String(), nil
		}
		if ch != '\r' {
			b.WriteByte(ch)
		}
	}
	return "", fmt.Errorf("input is too large")
}

func showStatus(cfg *config.Config) {
	fmt.Println()
	fmt.Println("  Loading usage data...")

	client := api.NewClient(cfg)
	usage, err := client.GetCurrentUsage(context.Background(), cfg)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}

	printUsage(usage)
}

func showHistory(cfg *config.Config) {
	fmt.Println()
	fmt.Println("  ═════════════════════════════════════════════════════════════")
	fmt.Println("                    Usage History")
	fmt.Println("  ═════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  Feature coming soon!")
}

func printUsage(u *models.CurrentUsage) {
	fmt.Println()
	fmt.Println("  ╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("  ║  %-53s║\n", "MiniMax M2.7 Usage")
	fmt.Println("  ╠════════════════════════════════════════════════════════════╣")
	fmt.Println("  ║  5-Hour Rolling Window (M2.7 Requests)                   ║")
	fmt.Println("  ╠════════════════════════════════════════════════════════════╣")
	fmt.Printf("  ║  Used:      %-6d / %-6d (%5.1f%%)%-21s║\n", u.WindowUsed, u.WindowLimit, u.WindowPercentUsed, "")
	fmt.Printf("  ║  Remaining: %-6d requests%-32s║\n", u.WindowRemaining, "")
	if u.WindowStart != "" && u.WindowEnd != "" {
		fmt.Printf("  ║  Window:    %s - %s║\n", u.WindowStart, u.WindowEnd)
	}
	fmt.Println("  ╠════════════════════════════════════════════════════════════╣")
	fmt.Println("  ║  Weekly Limit                                                ║")
	fmt.Println("  ╠════════════════════════════════════════════════════════════╣")
	fmt.Printf("  ║  Used:      %-6d / %-6d (%5.1f%%)%-21s║\n", u.WeeklyUsed, u.WeeklyLimit, u.WeeklyPercentUsed, "")
	fmt.Printf("  ║  Remaining: %-6d requests%-32s║\n", u.WeeklyRemaining, "")
	fmt.Println("  ╚════════════════════════════════════════════════════════════╝")
	fmt.Printf("  Last updated: %s\n", u.LastUpdated.Format("2006-01-02 15:04:05"))
}
