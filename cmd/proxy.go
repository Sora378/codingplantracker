package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sora378/codingplantracker/internal/config"
	"github.com/Sora378/codingplantracker/internal/proxy"
	"github.com/spf13/cobra"
)

var proxyPort int

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Run a local API proxy that records token usage",
	Long: `Run a localhost proxy for OpenAI-compatible API calls.
Point your client at http://127.0.0.1:<port> and Coplanage will forward
requests to your configured provider while recording token usage responses.`,
	RunE: runProxy,
}

func init() {
	proxyCmd.Flags().IntVarP(&proxyPort, "port", "p", 11434, "localhost port for the tracking proxy")
	rootCmd.AddCommand(proxyCmd)
}

func runProxy(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadFrom(cfgFile)
	if err != nil {
		cfg = config.DefaultConfigFrom(cfgFile)
	}
	if !cfg.IsLoggedIn() {
		return fmt.Errorf("not logged in; run 'cpq --cli' first")
	}

	p := proxy.NewProxy(cfg.ConfigDir(), proxyPort)
	if err := p.Start(); err != nil {
		return err
	}
	defer p.Stop()

	fmt.Printf("Coplanage proxy running on http://127.0.0.1:%d\n", proxyPort)
	fmt.Println("Press Ctrl+C to stop.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	return nil
}
