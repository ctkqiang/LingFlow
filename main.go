package main

type RuntimeMode int

const (
	Serverless RuntimeMode = iota
	Server
)

func main() {}

func Application(mode RuntimeMode) {
}
