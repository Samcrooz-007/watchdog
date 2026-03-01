package watchdog

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"notashelf.dev/watchdog/internal/config"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "watchdog",
	Short: "Privacy-first web analytics with Prometheus metrics",
	Long:  `Watchdog is a lightweight, privacy-first analytics system that aggregates web traffic data.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return Run(cfg)
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().
		StringVar(&cfgFile, "config", "", "config file (default: config.yaml)")
	rootCmd.PersistentFlags().String("listen-addr", "", "server listen address (overrides config)")
	rootCmd.PersistentFlags().String("metrics-path", "", "metrics endpoint path (overrides config)")
	rootCmd.PersistentFlags().
		String("ingestion-path", "", "ingestion endpoint path (overrides config)")

	// Bind flags to viper
	viper.BindPFlag("server.listen_addr", rootCmd.PersistentFlags().Lookup("listen-addr"))
	viper.BindPFlag("server.metrics_path", rootCmd.PersistentFlags().Lookup("metrics-path"))
	viper.BindPFlag("server.ingestion_path", rootCmd.PersistentFlags().Lookup("ingestion-path"))
}

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in current directory
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/watchdog")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// Environment variables
	viper.SetEnvPrefix("WATCHDOG")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Fprintf(
				os.Stderr,
				"Config file not found, using defaults and environment variables\n",
			)
		} else {
			fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
			os.Exit(1)
		}
	}

	// Unmarshal into config struct
	cfg = &config.Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse config: %v\n", err)
		os.Exit(1)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Config validation failed: %v\n", err)
		os.Exit(1)
	}
}

func Main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
