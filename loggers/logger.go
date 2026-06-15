package loggers

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/gj1118/faker/constants"
	"github.com/gj1118/faker/models"
)

func Init(loggingConfig models.LogConfig) {
	if !loggingConfig.Enabled {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return
	}

	var writer io.Writer = os.Stdout
	if loggingConfig.Where == "file" {
		rotateLogFile()
		file, err := os.OpenFile(constants.LOG_FILE_NAME, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal(err)
		}
		writer = file
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(loggingConfig.Info)); err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if loggingConfig.Handler == "text" {
		handler = slog.NewTextHandler(writer, opts)
	} else {
		handler = slog.NewJSONHandler(writer, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func rotateLogFile() {
	if _, err := os.Stat(constants.LOG_FILE_NAME); os.IsNotExist(err) {
		return
	}
	ts := time.Now().Format("20060102_150405")
	archived := fmt.Sprintf("faker_%s.log", ts)
	if err := os.Rename(constants.LOG_FILE_NAME, archived); err != nil {
		log.Printf("warning: could not rotate log file: %v", err)
	}
}


