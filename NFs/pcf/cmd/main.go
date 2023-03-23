/*
 * Npcf_BDTPolicyControl Service API
 *
 * The Npcf_BDTPolicyControl Service is used by an NF service consumer to
 * retrieve background data transfer policies from the PCF and to update the PCF with
 * the background data transfer policy selected by the NF service consumer.
 *
 * API version: 1.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/asaskevich/govalidator"
	"github.com/urfave/cli"

	"github.com/free5gc/pcf/internal/logger"
	"github.com/free5gc/pcf/internal/util"
	"github.com/free5gc/pcf/pkg/service"
	"github.com/free5gc/util/version"
)

var PCF = &service.PCF{}

func main() {
	defer func() {
		if p := recover(); p != nil {
			// Print stack for panic to log. Fatalf() will let program exit.
			logger.AppLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
		}
	}()

	app := cli.NewApp()
	app.Name = "pcf"
	app.Usage = "5G Policy Control Function (PCF)"
	app.Action = action
	app.Flags = PCF.GetCliCmd()
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("PCF Run Error: %v\n", err)
	}
}

func action(c *cli.Context) error {
	if err := initLogFile(c.String("log"), c.String("log5gc")); err != nil {
		logger.AppLog.Errorf("%+v", err)
		return err
	}

	if err := PCF.Initialize(c); err != nil {
		switch err1 := err.(type) {
		case govalidator.Errors:
			errs := err1.Errors()
			for _, e := range errs {
				logger.CfgLog.Errorf("%+v", e)
			}
		default:
			logger.CfgLog.Errorf("%+v", err)
		}

		logger.CfgLog.Errorf("[-- PLEASE REFER TO SAMPLE CONFIG FILE COMMENTS --]")
		return fmt.Errorf("Failed to initialize !!")
	}
	logger.AppLog.Infoln(c.App.Name)
	logger.AppLog.Infoln("PCF version: ", version.GetVersion())

	PCF.Start()

	return nil
}

func initLogFile(logNfPath, log5gcPath string) error {
	PCF.KeyLogPath = util.PcfDefaultKeyLogPath

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
		_, name := filepath.Split(util.PcfDefaultKeyLogPath)
		PCF.KeyLogPath = filepath.Join(tmpDir, name)
	}

	return nil
}
