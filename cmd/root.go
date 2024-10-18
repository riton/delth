/*
Copyright © 2024 Rémi Ferrand

Contributor(s): Rémi Ferrand <riton.github_at_gmail.com>, 2024

This software is governed by the CeCILL license under French law and
abiding by the rules of distribution of free software.  You can  use,
modify and/ or redistribute the software under the terms of the CeCILL
license as circulated by CEA, CNRS and INRIA at the following URL
"http://www.cecill.info".

As a counterpart to the access to the source code and  rights to copy,
modify and redistribute granted by the license, users are provided only
with a limited warranty  and the software's author,  the holder of the
economic rights,  and the successive licensors  have only  limited
liability.

In this respect, the user's attention is drawn to the risks associated
with loading,  using,  modifying and/or developing or reproducing the
software by the user in light of its specific status of free software,
that may mean  that it is complicated to manipulate,  and  that  also
therefore means  that it is reserved for developers  and  experienced
professionals having in-depth computer knowledge. Users are therefore
encouraged to load and test the software's suitability as regards their
requirements in conditions enabling the security of their systems and/or
data to be ensured and,  more generally, to use and operate it in the
same conditions as regards security.

The fact that you are presently reading this means that you have had
knowledge of the CeCILL license and that you accept its terms.
*/
package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "delth",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
	RunE:         rootCmdRunE,
	SilenceUsage: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.delth.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	viper.BindPFlag("debug", rootCmd.Flags().Lookup("debug"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.SetEnvPrefix("DELTH")

	viper.AutomaticEnv() // read in environment variables that match
}

func setupSigHandlers(ctx context.Context) context.Context {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	nctx, nctxCancel := context.WithCancel(ctx)

	go func() {
		sig := <-sigs
		slog.Debug("received signal", "signal", sig.String(), "component", "main")
		nctxCancel()
	}()

	return nctx
}

type healthCheckProxyConfig struct {
	Scheme     string `mapstructure:"scheme"`
	ListenAddr string `mapstructure:"listen_addr"`
}

type backendHealthCheckConfig struct {
	Path                  string        `mapstructure:"path" validate:"required"`
	Port                  int           `mapstructure:"port" validate:"required"`
	Scheme                string        `mapstructure:"scheme"`
	TLSInsecureSkipVerify bool          `mapstructure:"tls-insecure-skip-verify"`
	HTTPTimeout           time.Duration `mapstructure:"timeout"`
}

type commandExecConfig struct {
	ShutdownDelay time.Duration `mapstructure:"shutdown_delay"`
}

type config struct {
	HealthCheckProxy   healthCheckProxyConfig   `mapstructure:"healthcheck-proxy"`
	BackendHealthCheck backendHealthCheckConfig `mapstructure:"backend-healthcheck"`
	CommandExec        commandExecConfig        `mapstructure:"cmd-exec"`
}

func rootCmdRunE(cmd *cobra.Command, args []string) error {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	log := slog.With("component", "main")

	if viper.GetBool("debug") {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// our default configuration
	cfg := config{
		BackendHealthCheck: backendHealthCheckConfig{
			Scheme:      "http",
			HTTPTimeout: 30 * time.Second,
		},
		HealthCheckProxy: healthCheckProxyConfig{
			ListenAddr: ":8069",
			Scheme:     "http",
		},
		CommandExec: commandExecConfig{
			ShutdownDelay: 30 * time.Second,
		},
	}

	// WARNING:'-tags=viper_bind_struct' MUST be passed
	// to 'go run' / 'go build' for this Unmarshal() to consider
	// environment variables
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Error("unmarshaling configuration")
		return err
	}

	cfgValidator := validator.New()
	if err := cfgValidator.Struct(&cfg); err != nil {
		log.Error("missing required configuration")
		return err
	}

	log.Debug("delth configuration", "config", cfg)

	sigCtx := setupSigHandlers(rootCtx)

	proxy := NewHealthCheckProxy(sigCtx, HealthCheckProxyOptions{
		RealHealthCheckPath:   cfg.BackendHealthCheck.Path,
		RealHealthCheckPort:   cfg.BackendHealthCheck.Port,
		RealHealthCheckScheme: cfg.BackendHealthCheck.Scheme,
	})

	hClient := &http.Client{
		Timeout: cfg.BackendHealthCheck.HTTPTimeout,
	}

	if cfg.BackendHealthCheck.TLSInsecureSkipVerify {
		hClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	proxy.SetHTTPClient(hClient)

	mux := http.NewServeMux()
	mux.Handle("/delth/health", http.HandlerFunc(proxy.HealthHandler))

	srv := http.Server{
		Addr: cfg.HealthCheckProxy.ListenAddr,
		BaseContext: func(net.Listener) context.Context {
			return sigCtx
		},
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("serving HTTP requests", "component", "http-server", "error", err)
		}
	}()

	var globalExitErr error = nil

	cmdWrapper := NewCmdExecutor(sigCtx, args[0], args[1:]...)
	cmdWrapper.SetOnCmdFailureCb(func(err *exec.ExitError) {
		log.Debug("detected command failure, canceling root context")
		rootCancel()
		globalExitErr = fmt.Errorf("command has failed: %w", err)
	})

	if err := cmdWrapper.Start(); err != nil {
		return fmt.Errorf("starting command: %w", err)
	}

	<-sigCtx.Done()

	log.Debug("initiating proxy shutdown")

	proxy.InitiateShutdown()

	log.Debug("delaying process shutdown")

	time.Sleep(cfg.CommandExec.ShutdownDelay)

	log.Debug("delay expired, killing process")

	cmdWrapper.Stop()

	shutdownCtx, shutdownCancelFn := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancelFn()

	log.Debug("shutting down HTTP server")

	if err := srv.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
		log.Error("shutting down HTTP server", "error", err.Error())
	}

	log.Debug("HTTP server is stopped")

	return globalExitErr
}
