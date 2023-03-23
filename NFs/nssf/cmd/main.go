package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/urfave/cli"

	"github.com/free5gc/nssf/internal/logger"
	"github.com/free5gc/nssf/internal/util"
	"github.com/free5gc/nssf/pkg/service"
	"github.com/free5gc/util/version"
)

var NSSF = &service.NSSF{}

func main() {
	defer func() {
		if p := recover(); p != nil {
			// Print stack for panic to log. Fatalf() will let program exit.
			logger.AppLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
		}
	}()

	app := cli.NewApp()
	app.Name = "nssf"
	app.Usage = "5G Network Slice Selection Function (NSSF)"
	app.Action = action
	app.Flags = NSSF.GetCliCmd()
	if err := app.Run(os.Args); err != nil {
		logger.AppLog.Errorf("NSSF Run Error: %v\n", err)
	}
}

func action(c *cli.Context) error {
	if err := initLogFile(c.String("log"), c.String("log5gc")); err != nil {
		logger.AppLog.Errorf("%+v", err)
		return err
	}

	if err := NSSF.Initialize(c); err != nil {
		logger.CfgLog.Errorf("%+v", err)
		return fmt.Errorf("Failed to initialize !!")
	}

	logger.AppLog.Infoln(c.App.Name)
	logger.AppLog.Infoln("NSSF version: ", version.GetVersion())

	NSSF.Start()

	return nil
}

func initLogFile(logNfPath, log5gcPath string) error {
	NSSF.KeyLogPath = util.NssfDefaultKeyLogPath

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
		_, name := filepath.Split(util.NssfDefaultKeyLogPath)
		NSSF.KeyLogPath = filepath.Join(tmpDir, name)
	}

	return nil
}
