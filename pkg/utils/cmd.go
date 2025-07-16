package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	commonio "github.com/longhorn/go-common-libs/io"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
)

// SetGlobalOptionsLocal sets global options for local commands.
func SetGlobalOptionsLocal(cmd *cobra.Command, globalOpts *types.GlobalCmdOptions) {
	cmd.PersistentFlags().StringVarP(&globalOpts.LogLevel, consts.CmdOptLogLevel, "l", globalOpts.LogLevel, "Log level")
}

// SetGlobalOptionsRemote sets global options for remote commands.
func SetGlobalOptionsRemote(cmd *cobra.Command, globalOpts *types.GlobalCmdOptions) {
	cmd.PersistentFlags().StringVarP(&globalOpts.LogLevel, consts.CmdOptLogLevel, "l", globalOpts.LogLevel, "Log level")
	cmd.PersistentFlags().StringVar(&globalOpts.KubeConfigPath, consts.CmdOptKubeConfigPath, globalOpts.KubeConfigPath, "Kubernetes config (kubeconfig) path")
	cmd.PersistentFlags().StringVar(&globalOpts.Image, consts.CmdOptImage, globalOpts.Image, "Image containing longhornctl-local")
	cmd.PersistentFlags().StringVar(&globalOpts.NodeSelector, consts.CmdOptNodeSelector, globalOpts.NodeSelector, "Comma-separated list of key=value pairs to match against node labels, selecting the nodes the DaemonSet will run on (e.g. env=prod,zone=us-west).")
	cmd.PersistentFlags().StringVar(&globalOpts.Namespace, consts.CmdOptNamespace, globalOpts.Namespace, "The namespace to run DaemonSet pods.")
}

// SetFlagHidden adds a option flag to the given command and mark it as hidden.
// This is useful for hiding flags that are not meant to be used or are not intended
// to be exposed to users via the command-line help menus.
func SetFlagHidden(cmd *cobra.Command, option string) {
	cmd.Flags().String(option, "", "")

	if err := cmd.Flags().MarkHidden(option); err != nil {
		logrus.WithError(err).Warnf("Failed to mark option %s as hidden", option)
	}
}

// SetLog initializes logrus.
// It sets log level and timestamp format.
func SetLog(logLevel string) error {
	if err := setLogLevel(logLevel); err != nil {
		return err
	}

	// The default log formatter shows like this: INFO[0000].
	// Set it to show full timestamp to give more information.
	isFullTimestamp := true
	setLogFormatter(isFullTimestamp)

	logrus.WithFields(logrus.Fields{
		"level":          logLevel,
		"full-timestamp": isFullTimestamp,
	}).Trace("Initialized logger")

	return nil
}

func setLogLevel(logLevel string) error {
	logrusLevel, err := logrus.ParseLevel(logLevel)
	if err != nil {
		return err
	}

	logrus.SetLevel(logrusLevel)
	return nil
}

func setLogFormatter(isFullTimestamp bool) {
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: isFullTimestamp})
}

// CheckErr logs the error and exits with a non-zero code.
// This is similar to cobra.CheckErr except that it prints with logrus.
func CheckErr(err error) {
	if err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
}

// ConvertStringToTypeOrDefault converts a string to the type of the default value.
// If string is empty, it will return the default value.
// If string is not empty, it will attempt to convert it to the type of the default value.
// If conversion fails, it will return the default value.
func ConvertStringToTypeOrDefault[T any](str string, defaultValue T) T {
	if str == "" {
		return defaultValue
	}

	var value T
	valueType := reflect.TypeOf(defaultValue).Kind()

	switch valueType {
	case reflect.Int:
		intValue, err := strconv.Atoi(str)
		if err != nil {
			logrus.WithError(err).Warn("Failed to convert string to integer")
		} else {
			value = reflect.ValueOf(intValue).Interface().(T)
		}

	case reflect.Bool:
		boolValue, err := strconv.ParseBool(str)
		if err != nil {
			logrus.WithError(err).Warn("Failed to convert string to boolean")
		} else {
			value = reflect.ValueOf(boolValue).Interface().(T)
		}

	default:
		logrus.WithField("type", reflect.TypeOf(defaultValue)).Warn("Unsupported default value type")
		return defaultValue
	}

	return value
}

func HandleResult(resultBytes []byte, outputFile string, logger *logrus.Entry) error {
	if len(outputFile) == 0 {
		fmt.Printf("Result: \n%s\n", resultBytes)
		return nil
	}

	// Create directory if not already created.
	_, err := commonio.CreateDirectory(filepath.Dir(outputFile), time.Now())
	if err != nil {
		return errors.Wrap(err, "failed to create directory")
	}

	// Add trailing end of line.
	resultBytes = append(resultBytes, '\n')

	// Create output file.
	logger.Debug("Writing result to file")
	err = os.WriteFile(outputFile, resultBytes, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to write output file")
	}

	return nil
}
