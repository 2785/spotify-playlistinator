package cmd

import (
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var rootCmd = &cobra.Command{
	Use:   "spotify-playlistinator",
	Short: "Util to edit playlists in bulk",
}

func Execute() {
	loggerConf := zap.NewDevelopmentConfig()
	loggerConf.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
	loggerConf.EncoderConfig.EncodeCaller = nil

	if isatty.IsTerminal(os.Stdout.Fd()) {
		loggerConf.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := loggerConf.Build()

	cobra.CheckErr(err)

	zap.ReplaceGlobals(logger)

	cobra.CheckErr(rootCmd.Execute())
}

func init() {}
