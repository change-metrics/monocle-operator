/*
Copyright 2023 Monocle developers.

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

package main

import (
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	monoclev1alpha1 "github.com/change-metrics/monocle-operator/api/v1alpha1"
	"github.com/change-metrics/monocle-operator/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(monoclev1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// getWatchNamespace returns the Namespace the operator should be watching for changes
func getWatchNamespace() (string, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	return ns, nil
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var monocleResource string
	var monocleNamespace string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&monocleResource, "cr", "", "standalone mode: the custom resource to reconcile.")
	flag.StringVar(&monocleNamespace, "ns", "", "standalone mode: the namespace where to reconcile the CR.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var standalone = false
	if (monocleResource != "" && monocleNamespace == "") || (monocleResource == "" && monocleNamespace != "") {
		setupLog.Info("When running the standalon mode, both --cr and --ns option must be set")
		os.Exit(1)
	} else if monocleResource != "" && monocleNamespace != "" {
		standalone = true
	}

	if standalone {
		var mr monoclev1alpha1.Monocle
		cl, err := client.New(ctrl.GetConfigOrDie(), client.Options{})
		if err != nil {
			setupLog.Error(err, "unable to create a client")
			os.Exit(1)
		}
		reconcilier := controllers.MonocleReconciler{
			Client: cl,
			Scheme: scheme,
		}
		dat, err := os.ReadFile(monocleResource)
		if err != nil {
			panic(err.Error())
		}
		if err := yaml.Unmarshal(dat, &mr); err != nil {
			panic(err.Error())
		}
		setupLog.Info("Standalone reconciling from CR passed by parameter",
			"CR", monocleResource,
			"CR name", mr.ObjectMeta.Name,
			"NS", monocleNamespace)
		ctx := ctrl.SetupSignalHandler()
		reconcilier.StandaloneReconcile(ctx, monocleNamespace, mr)
		os.Exit(0)
	} else {
		watchNamespace, err := getWatchNamespace()
		if err != nil {
			setupLog.Error(err, "unable to get WatchNamespace, "+
				"the manager will watch and manage resources in all namespaces")
		}
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     metricsAddr,
			Port:                   9443,
			Namespace:              watchNamespace,
			HealthProbeBindAddress: probeAddr,
			LeaderElection:         enableLeaderElection,
			LeaderElectionID:       "0a3d6799.monocle.change-metrics.io",
		})
		if err != nil {
			setupLog.Error(err, "unable to start manager")
			os.Exit(1)
		}
		reconcilier := controllers.MonocleReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		if err = reconcilier.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Monocle")
			os.Exit(1)
		}
		if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up health check")
			os.Exit(1)
		}
		if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up ready check")
			os.Exit(1)
		}
		setupLog.Info("starting manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			setupLog.Error(err, "problem running manager")
			os.Exit(1)
		}
	}
}
