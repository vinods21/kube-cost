package main

import "github.com/kube-cost/kube-cost/internal/operatorentry"

func main() {
	operatorentry.Run("platform-operator", "platform-operator.cost.kube-cost.io")
}
