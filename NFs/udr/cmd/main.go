package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/asaskevich/govalidator"
	"github.com/urfave/cli"

	"github.com/free5gc/udr/internal/logger"
	"github.com/free5gc/udr/internal/util"
	udr_service "github.com/free5gc/udr/pkg/service"
	"github.com/free5gc/util/version"
)

var UDR = &udr_service.UDR{}

func main() {
	defer func() {
		if p := recover(); p != nil {
			// Print stack for panic to log. Fatalf() will let program exit.
			logger.AppLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
		}
	}()

	app := cli.NewApp()
	app.Name = "udr"
	app.Usage = "5G Unified Data Repository (UDR)"
	app.Action = action
	app.Flags = UDR.GetCliCmd()
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("UDR Run error: %v\n", err)
	}
}

func action(c *cli.Context) error {
	if err := initLogFile(c.String("log"), c.String("log5gc")); err != nil {
		logger.AppLog.Errorf("%+v", err)
		return err
	}

	if err := UDR.Initialize(c); err != nil {
		switch errType := err.(type) {
		case govalidator.Errors:
			validErrs := err.(govalidator.Errors).Errors()
			for _, validErr := range validErrs {
				logger.CfgLog.Errorf("%+v", validErr)
			}
		default:
			logger.CfgLog.Errorf("%+v", errType)
		}
		logger.CfgLog.Errorf("[-- PLEASE REFER TO SAMPLE CONFIG FILE COMMENTS --]")
		return fmt.Errorf("Failed to initialize !!")
	}

	logger.AppLog.Infoln(c.App.Name)
	logger.AppLog.Infoln("UDR version: ", version.GetVersion())

	UDR.Start()

	return nil
}

func initLogFile(logNfPath, log5gcPath string) error {
	UDR.KeyLogPath = util.UdrDefaultKeyLogPath

	if err := logger.LogFileHook(logNfPath, log5gcPath); err != nil {
		return err
	}

	if logNfPath != "" {
		nfDir, _ := filepath.Split(logNfPath)
		tmpDir := filepath.Join(nfDir, "key")
		if err := os.MkdirAll(tmpDir, 0775); err != nil {
			logger.InitLog.Errorf("Make directory %s failed: %+v", tmpDir, err)
			return err
		}
		_, name := filepath.Split(util.UdrDefaultKeyLogPath)
		UDR.KeyLogPath = filepath.Join(tmpDir, name)
	}

	return nil
}
