package hermes

type Int interface {
	GetScore(lastOps int) (score float64)
	GetMicsecsBetweenOps(lastOps int) float64
	GetNumOps() int
	StartPlaying()
	StopPlaying()
	IsPlaying() bool
}
