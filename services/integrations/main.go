package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kube-cost/kube-cost/internal/serviceentry"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	address := os.Getenv("KARPENTER_HTTP_ADDRESS")
	if address == "" {
		serviceentry.Run("integrations")
		return
	}
	if err := runKarpenterAPI(address); err != nil {
		slog.Error("karpenter integration stopped", "error", err)
		os.Exit(1)
	}
}

func runKarpenterAPI(address string) error {
	config, err := kubernetesConfig()
	if err != nil {
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	coreClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              address,
		Handler:           NewAPI(NewDynamicReader(os.Getenv("CLUSTER_ID"), dynamicClient, coreClient), Scorer{}).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("karpenter API shutdown failed", "error", err)
		}
	}()
	slog.Info("karpenter integration API listening", "address", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func kubernetesConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if home := os.Getenv("USERPROFILE"); home != "" {
		kubeconfig := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kubeconfig); err == nil {
			return clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		kubeconfig := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kubeconfig); err == nil {
			return clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
	}
	return rest.InClusterConfig()
}
