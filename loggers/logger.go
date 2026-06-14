package loggers

import (
	"io"
	"log"
	"log/slog"
	"os"

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
