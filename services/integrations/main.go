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
	controlAddress := os.Getenv("INTEGRATIONS_HTTP_ADDRESS")
	address := os.Getenv("KARPENTER_HTTP_ADDRESS")
	if controlAddress != "" && address != "" {
		if err := runCombinedAPI(controlAddress); err != nil {
			slog.Error("integrations API stopped", "error", err)
			os.Exit(1)
		}
		return
	}
	if controlAddress != "" {
		runControlAPI(controlAddress)
		return
	}
	if address == "" {
		serviceentry.Run("integrations")
		return
	}
	if err := runKarpenterAPI(address); err != nil {
		slog.Error("karpenter integration stopped", "error", err)
		os.Exit(1)
	}
}

func runControlAPI(address string) {
	api := NewControlAPI(NewIntegrationStore(), time.Now)
	server := &http.Server{
		Addr:              address,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	runHTTPServer("integrations control API", server)
}

func runCombinedAPI(address string) error {
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
	controlAPI := NewControlAPI(NewIntegrationStore(), time.Now)
	karpenterAPI := NewAPI(NewDynamicReader(os.Getenv("CLUSTER_ID"), dynamicClient, coreClient), Scorer{})
	mux := http.NewServeMux()
	mux.Handle("/", controlAPI.Routes())
	mux.Handle("GET /api/v1/karpenter/snapshot", karpenterAPI.Routes())
	mux.Handle("GET /api/v1/karpenter/scores", karpenterAPI.Routes())
	server := &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	runHTTPServer("integrations combined API", server)
	return nil
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
	runHTTPServer("karpenter integration API", server)
	return nil
}

func runHTTPServer(name string, server *http.Server) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error(name+" shutdown failed", "error", err)
		}
	}()
	slog.Info(name+" listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error(name+" stopped", "error", err)
		os.Exit(1)
	}
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
