/*
 * NSSF Service
 */

package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/free5gc/nssf/internal/context"
	"github.com/free5gc/nssf/internal/logger"
	"github.com/free5gc/nssf/internal/sbi/consumer"
	"github.com/free5gc/nssf/internal/sbi/nssaiavailability"
	"github.com/free5gc/nssf/internal/sbi/nsselection"
	"github.com/free5gc/nssf/internal/util"
	"github.com/free5gc/nssf/pkg/factory"
	"github.com/free5gc/util/httpwrapper"
	logger_util "github.com/free5gc/util/logger"
)

type NSSF struct {
	KeyLogPath string
}

type (
	// Commands information.
	Commands struct {
		config string
	}
)

var commands Commands

var cliCmd = []cli.Flag{
	cli.StringFlag{
		Name:  "config, c",
		Usage: "Load configuration from `FILE`",
	},
	cli.StringFlag{
		Name:  "log, l",
		Usage: "Output NF log to `FILE`",
	},
	cli.StringFlag{
		Name:  "log5gc, lc",
		Usage: "Output free5gc log to `FILE`",
	},
}

func (*NSSF) GetCliCmd() (flags []cli.Flag) {
	return cliCmd
}

func (nssf *NSSF) Initialize(c *cli.Context) error {
	commands = Commands{
		config: c.String("config"),
	}

	if commands.config != "" {
		if err := factory.InitConfigFactory(commands.config); err != nil {
			return err
		}
	} else {
		if err := factory.InitConfigFactory(util.NssfDefaultConfigPath); err != nil {
			return err
		}
	}

	nssf.SetLogLevel()

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	return nil
}

func (nssf *NSSF) SetLogLevel() {
	if factory.NssfConfig.Logger == nil {
		logger.InitLog.Warnln("NSSF config without log level setting!!!")
		return
	}

	if factory.NssfConfig.Logger.NSSF != nil {
		if factory.NssfConfig.Logger.NSSF.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.NssfConfig.Logger.NSSF.DebugLevel); err != nil {
				logger.InitLog.Warnf("NSSF Log level [%s] is invalid, set to [info] level",
					factory.NssfConfig.Logger.NSSF.DebugLevel)
				logger.SetLogLevel(logrus.InfoLevel)
			} else {
				logger.InitLog.Infof("NSSF Log level is set to [%s] level", level)
				logger.SetLogLevel(level)
			}
		} else {
			logger.InitLog.Infoln("NSSF Log level not set. Default set to [info] level")
			logger.SetLogLevel(logrus.InfoLevel)
		}
		logger.SetReportCaller(factory.NssfConfig.Logger.NSSF.ReportCaller)
	}
}

func (nssf *NSSF) FilterCli(c *cli.Context) (args []string) {
	for _, flag := range nssf.GetCliCmd() {
		name := flag.GetName()
		value := fmt.Sprint(c.Generic(name))
		if value == "" {
			continue
		}

		args = append(args, "--"+name, value)
	}
	return args
}

func (nssf *NSSF) Start() {
	logger.InitLog.Infoln("Server started")

	router := logger_util.NewGinWithLogrus(logger.GinLog)

	nssaiavailability.AddService(router)
	nsselection.AddService(router)

	pemPath := util.NssfDefaultPemPath
	keyPath := util.NssfDefaultKeyPath
	sbi := factory.NssfConfig.Configuration.Sbi
	if sbi.Tls != nil {
		pemPath = sbi.Tls.Pem
		keyPath = sbi.Tls.Key
	}

	self := context.NSSF_Self()
	context.InitNssfContext()
	addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

	// Register to NRF
	profile, err := consumer.BuildNFProfile(self)
	if err != nil {
		logger.InitLog.Error("Failed to build NSSF profile")
	}
	_, self.NfId, err = consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, profile)
	if err != nil {
		logger.InitLog.Errorf("Failed to register NSSF to NRF: %s", err.Error())
	}

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				// Print stack for panic to log. Fatalf() will let program exit.
				logger.InitLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
			}
		}()

		<-signalChannel
		nssf.Terminate()
		os.Exit(0)
	}()

	server, err := httpwrapper.NewHttp2Server(addr, nssf.KeyLogPath, router)

	if server == nil {
		logger.InitLog.Errorf("Initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		logger.InitLog.Warnf("Initialize HTTP server: +%v", err)
	}

	serverScheme := factory.NssfConfig.Configuration.Sbi.Scheme
	if serverScheme == "http" {
		err = server.ListenAndServe()
	} else if serverScheme == "https" {
		err = server.ListenAndServeTLS(pemPath, keyPath)
	}

	if err != nil {
		logger.InitLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (nssf *NSSF) Exec(c *cli.Context) error {
	logger.InitLog.Traceln("args:", c.String("nssfcfg"))
	args := nssf.FilterCli(c)
	logger.InitLog.Traceln("filter: ", args)
	command := exec.Command("./nssf", args...)

	stdout, err := command.StdoutPipe()
	if err != nil {
		logger.InitLog.Fatalln(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				// Print stack for panic to log. Fatalf() will let program exit.
				logger.InitLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
			}
		}()

		in := bufio.NewScanner(stdout)
		for in.Scan() {
			fmt.Println(in.Text())
		}
		wg.Done()
	}()

	stderr, err := command.StderrPipe()
	if err != nil {
		logger.InitLog.Fatalln(err)
	}
	go func() {
		defer func() {
			if p := recover(); p != nil {
				// Print stack for panic to log. Fatalf() will let program exit.
				logger.InitLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
			}
		}()

		in := bufio.NewScanner(stderr)
		for in.Scan() {
			fmt.Println(in.Text())
		}
		wg.Done()
	}()

	go func() {
		defer func() {
			if p := recover(); p != nil {
				// Print stack for panic to log. Fatalf() will let program exit.
				logger.InitLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
			}
		}()

		if err = command.Start(); err != nil {
			fmt.Printf("NSSF Start error: %v", err)
		}
		wg.Done()
	}()

	wg.Wait()

	return err
}

func (nssf *NSSF) Terminate() {
	logger.InitLog.Infof("Terminating NSSF...")
	// deregister with NRF
	problemDetails, err := consumer.SendDeregisterNFInstance()
	if problemDetails != nil {
		logger.InitLog.Errorf("Deregister NF instance Failed Problem[%+v]", problemDetails)
	} else if err != nil {
		logger.InitLog.Errorf("Deregister NF instance Error[%+v]", err)
	} else {
		logger.InitLog.Infof("Deregister from NRF successfully")
	}

	logger.InitLog.Infof("NSSF terminated")
}
