package main

import "github.com/kube-cost/kube-cost/internal/operatorentry"

func main() {
	operatorentry.Run("action-executor-operator", "action-executor.cost.kube-cost.io")
}
