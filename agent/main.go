package main

import "github.com/kube-cost/kube-cost/internal/operatorentry"

func main() {
	operatorentry.Run("kubernetes-agent", "agent.cost.kube-cost.io")
}
