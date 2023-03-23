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

	ausf_context "github.com/free5gc/ausf/internal/context"
	"github.com/free5gc/ausf/internal/logger"
	"github.com/free5gc/ausf/internal/sbi/consumer"
	"github.com/free5gc/ausf/internal/sbi/ueauthentication"
	"github.com/free5gc/ausf/internal/util"
	"github.com/free5gc/ausf/pkg/factory"
	"github.com/free5gc/util/httpwrapper"
	logger_util "github.com/free5gc/util/logger"
)

type AUSF struct {
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

func (*AUSF) GetCliCmd() (flags []cli.Flag) {
	return cliCmd
}

func (ausf *AUSF) Initialize(c *cli.Context) error {
	commands = Commands{
		config: c.String("config"),
	}

	if commands.config != "" {
		if err := factory.InitConfigFactory(commands.config); err != nil {
			return err
		}
	} else {
		if err := factory.InitConfigFactory(util.AusfDefaultConfigPath); err != nil {
			return err
		}
	}

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	if _, err := factory.AusfConfig.Validate(); err != nil {
		return err
	}

	ausf.SetLogLevel()

	return nil
}

func (ausf *AUSF) SetLogLevel() {
	if factory.AusfConfig.Logger == nil {
		logger.InitLog.Warnln("AUSF config without log level setting!!!")
		return
	}

	if factory.AusfConfig.Logger.AUSF != nil {
		if factory.AusfConfig.Logger.AUSF.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AusfConfig.Logger.AUSF.DebugLevel); err != nil {
				logger.InitLog.Warnf("AUSF Log level [%s] is invalid, set to [info] level",
					factory.AusfConfig.Logger.AUSF.DebugLevel)
				logger.SetLogLevel(logrus.InfoLevel)
			} else {
				logger.InitLog.Infof("AUSF Log level is set to [%s] level", level)
				logger.SetLogLevel(level)
			}
		} else {
			logger.InitLog.Warnln("AUSF Log level not set. Default set to [info] level")
			logger.SetLogLevel(logrus.InfoLevel)
		}
		logger.SetReportCaller(factory.AusfConfig.Logger.AUSF.ReportCaller)
	}
}

func (ausf *AUSF) FilterCli(c *cli.Context) (args []string) {
	for _, flag := range ausf.GetCliCmd() {
		name := flag.GetName()
		value := fmt.Sprint(c.Generic(name))
		if value == "" {
			continue
		}

		args = append(args, "--"+name, value)
	}
	return args
}

func (ausf *AUSF) Start() {
	logger.InitLog.Infoln("Server started")

	router := logger_util.NewGinWithLogrus(logger.GinLog)
	ueauthentication.AddService(router)

	pemPath := util.AusfDefaultPemPath
	keyPath := util.AusfDefaultKeyPath
	sbi := factory.AusfConfig.Configuration.Sbi
	if sbi.Tls != nil {
		pemPath = sbi.Tls.Pem
		keyPath = sbi.Tls.Key
	}

	ausf_context.Init()
	self := ausf_context.GetSelf()
	// Register to NRF
	profile, err := consumer.BuildNFInstance(self)
	if err != nil {
		logger.InitLog.Error("Build AUSF Profile Error")
	}
	_, self.NfId, err = consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, profile)
	if err != nil {
		logger.InitLog.Errorf("AUSF register to NRF Error[%s]", err.Error())
	}

	addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

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
		ausf.Terminate()
		os.Exit(0)
	}()

	server, err := httpwrapper.NewHttp2Server(addr, ausf.KeyLogPath, router)
	if server == nil {
		logger.InitLog.Errorf("Initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		logger.InitLog.Warnf("Initialize HTTP server: +%v", err)
	}

	serverScheme := factory.AusfConfig.Configuration.Sbi.Scheme
	if serverScheme == "http" {
		err = server.ListenAndServe()
	} else if serverScheme == "https" {
		err = server.ListenAndServeTLS(pemPath, keyPath)
	}

	if err != nil {
		logger.InitLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (ausf *AUSF) Exec(c *cli.Context) error {
	logger.InitLog.Traceln("args:", c.String("ausfcfg"))
	args := ausf.FilterCli(c)
	logger.InitLog.Traceln("filter: ", args)
	command := exec.Command("./ausf", args...)

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

		startErr := command.Start()
		if startErr != nil {
			logger.InitLog.Fatalln(startErr)
		}
		wg.Done()
	}()

	wg.Wait()

	return err
}

func (ausf *AUSF) Terminate() {
	logger.InitLog.Infof("Terminating AUSF...")
	// deregister with NRF
	problemDetails, err := consumer.SendDeregisterNFInstance()
	if problemDetails != nil {
		logger.InitLog.Errorf("Deregister NF instance Failed Problem[%+v]", problemDetails)
	} else if err != nil {
		logger.InitLog.Errorf("Deregister NF instance Error[%+v]", err)
	} else {
		logger.InitLog.Infof("Deregister from NRF successfully")
	}

	logger.InitLog.Infof("AUSF terminated")
}
