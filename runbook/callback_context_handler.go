package runbook

type CallbackContextHandler interface {
	CreatePipe()
	Read()
	ClosePipe()
}
