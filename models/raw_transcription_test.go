package models_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/timpratim/Speech-To-Text/models"
)

func TestTranscriptions_ToMap(t *testing.T) {
	transcriptions := models.Transcriptions{
		{
			Index:     1,
			Text:      "Hello, World!",
			StartTime: "00:00:01",
			EndTime:   "00:00:02",
		},
		{
			Index:     2,
			Text:      "I am an AI.",
			StartTime: "00:00:03",
			EndTime:   "00:00:04",
		},
	}

	expectedMaps := []map[string]interface{}{
		{
			"index":     1,
			"text":      "Hello, World!",
			"startTime": "00:00:01",
			"endTime":   "00:00:02",
		},
		{
			"index":     2,
			"text":      "I am an AI.",
			"startTime": "00:00:03",
			"endTime":   "00:00:04",
		},
	}

	maps := transcriptions.ToMap()

	if !reflect.DeepEqual(maps, expectedMaps) {
		t.Errorf("Transcriptions.ToMap() = %v, want %v", maps, expectedMaps)
	}
}

func TestToModel(t *testing.T) {
	startTime1 := time.Duration(10) * time.Second
	endTime1 := time.Duration(15) * time.Second
	startTime2 := time.Duration(20) * time.Second
	endTime2 := time.Duration(25) * time.Second

	rawTranscriptions := &[]models.RawTranscription{
		{
			StartTs: startTime1,
			StopTs:  endTime1,
			Text:    "Hello",
			Index:   1,
		},
		{
			StartTs: startTime2,
			StopTs:  endTime2,
			Text:    "World",
			Index:   2,
		},
	}
	expected := models.Transcriptions{
		models.Transcription{
			StartTime: "10s",
			EndTime:   "15s",
			Text:      "Hello",
			Index:     1,
		},
		models.Transcription{
			StartTime: "20s",
			EndTime:   "25s",
			Text:      "World",
			Index:     2,
		},
	}

	models := models.ToModel(rawTranscriptions)

	assert.Equal(t, expected, models)
}
