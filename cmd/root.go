/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"os"
	"time"

	v1 "agones.dev/agones/pkg/apis/agones/v1"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/Octops/agones-event-broadcaster/pkg/broadcaster"
	"github.com/Octops/agones-event-broadcaster/pkg/brokers"
	"github.com/Octops/agones-event-broadcaster/pkg/brokers/kafka"
	"github.com/Octops/agones-event-broadcaster/pkg/brokers/pubsub"
	"github.com/Octops/agones-event-broadcaster/pkg/brokers/stdout"
)

var (
	cfgFile                 string
	kubeconfig              string
	verbose                 bool
	brokerFlag              string
	syncPeriod              string
	port                    int
	metricsBindAddress      string
	healthProbeBindAddress  string
	maxConcurrencyReconcile int
)

var rootCmd = &cobra.Command{
	Use:   "agones-event-broadcaster",
	Short: "Broadcast Events from Agones GameServers",
	Long:  `Broadcast Events from Agones GameServers`,
	Run: func(cmd *cobra.Command, args []string) {
		logrus.SetFormatter(&logrus.JSONFormatter{})
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}
		clientConf, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			logrus.WithError(err).Fatalf("error reading kubeconfig: %s", kubeconfig)
		}

		broker := BuildBroker(brokerFlag)

		duration, err := time.ParseDuration(syncPeriod)
		if err != nil {
			logrus.WithError(err).Fatalf("error parsing sync-period flag: %s", syncPeriod)
		}

		opts := &broadcaster.Config{
			SyncPeriod:             duration,
			ServerPort:             port,
			MetricsBindAddress:     metricsBindAddress,
			MaxConcurrentReconcile: 4,
			HealthProbeBindAddress: healthProbeBindAddress,
		}
		bc := broadcaster.New(clientConf, broker, opts)

		if err := bc.WithWatcherFor(&v1.Fleet{}).WithWatcherFor(&v1.GameServer{}).Build(); err != nil {
			logrus.WithError(err).Fatal("error creating broadcaster")
		}

		ctx := ctrl.SetupSignalHandler()
		if err := bc.Start(ctx); err != nil {
			logrus.WithError(err).Fatal("error starting broadcaster")
		}
	},
}

// BuildBroker creates a broker based on the broker flag.
// This will refactored in the future and will be placed on a package
func BuildBroker(ofType string) brokers.Broker {
	if ofType == "pubsub" {
		var opts []option.ClientOption
		// If the broadcaster is running within GCP, credentials don't need to be explicitly passed
		// Setting this environment variable is optional. The Service Accounts attached to the worker node should be able to perform the operation via IAM settings.
		if os.Getenv("PUBSUB_CREDENTIALS") != "" {
			opts = append(opts, option.WithCredentialsFile(os.Getenv("PUBSUB_CREDENTIALS")))
		}

		broker, err := pubsub.NewPubSubBroker(&pubsub.Config{
			ProjectID:       os.Getenv("PUBSUB_PROJECT_ID"),
			OnAddTopicID:    "agones.events.added",
			OnUpdateTopicID: "agones.events.updated",
			OnDeleteTopicID: "agones.events.deleted",
		}, opts...)
		if err != nil {
			logrus.WithError(err).Fatal("error creating broker")
		}

		return broker
	} else if ofType == "kafka" {
		broker, err := kafka.NewKafkaBroker(&kafka.Config{
			APIKey:           os.Getenv("KAFKA_APIKEY"),
			APISecret:        os.Getenv("KAFKA_APISECRET"),
			BootstrapServers: os.Getenv("KAFKA_SERVERS"),
		})
		if err != nil {
			logrus.WithError(err).Fatal("error creating kafka broker")
		}
		return broker
	}

	// Used only for debugging purpose
	return &stdout.StdoutBroker{}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.agones-event-broadcaster.yaml)")
	rootCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Set KUBECONFIG")
	rootCmd.Flags().StringVar(&brokerFlag, "broker", "", "The type of the broker to be used by the broadcaster")
	rootCmd.Flags().StringVar(&syncPeriod, "sync-period", "15s", "Determines the minimum frequency at which watched resources are reconciled")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Set log level to verbose, defaults to false")
	rootCmd.Flags().IntVarP(&port, "port", "p", 8089, "Port used by the broadcaster to communicate via http")
	rootCmd.Flags().StringVar(&metricsBindAddress, "metrics-bind-address", "0.0.0.0:8095", "The TCP address that the controller should bind to for serving prometheus metrics")
	rootCmd.Flags().IntVar(&maxConcurrencyReconcile, "max-concurrency", 5, "Maximum number of concurrent Reconciles which can be run")
	rootCmd.Flags().StringVar(&healthProbeBindAddress, "health-probe-bind-address", "0.0.0:8099", "The TCP address that the controller should bind to for serving health probes")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".agones-event-broadcaster" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".agones-event-broadcaster")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
