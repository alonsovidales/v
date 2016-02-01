package philoctetes

import (
	"testing"
)

func TestTrainer(t *testing.T) {
	GetTrainerCuda(
		"../test_data/training.log",
		10,
		3600*2,
	)
}
