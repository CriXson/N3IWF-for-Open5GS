package logger

import (
	"os"
	"time"

	formatter "github.com/antonfisher/nested-logrus-formatter"
	"github.com/sirupsen/logrus"

	logger_util "github.com/free5gc/util/logger"
)

var (
	log                 *logrus.Logger
	AppLog              *logrus.Entry
	InitLog             *logrus.Entry
	CfgLog              *logrus.Entry
	UeAuthPostLog       *logrus.Entry
	Auth5gAkaComfirmLog *logrus.Entry
	EapAuthComfirmLog   *logrus.Entry
	HandlerLog          *logrus.Entry
	ContextLog          *logrus.Entry
	ConsumerLog         *logrus.Entry
	GinLog              *logrus.Entry
)

func init() {
	log = logrus.New()
	log.SetReportCaller(false)

	log.Formatter = &formatter.Formatter{
		TimestampFormat: time.RFC3339,
		TrimMessages:    true,
		NoFieldsSpace:   true,
		HideKeys:        true,
		FieldsOrder:     []string{"component", "category"},
	}

	AppLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "App"})
	InitLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "Init"})
	CfgLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "CFG"})
	UeAuthPostLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "UeAuthPost"})
	Auth5gAkaComfirmLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "5gAkaAuth"})
	EapAuthComfirmLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "EapAkaAuth"})
	HandlerLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "Handler"})
	ContextLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "ctx"})
	ConsumerLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "Consumer"})
	GinLog = log.WithFields(logrus.Fields{"component": "AUSF", "category": "GIN"})
}

func LogFileHook(logNfPath string, log5gcPath string) error {
	if fullPath, err := logger_util.CreateFree5gcLogFile(log5gcPath); err == nil {
		if fullPath != "" {
			free5gcLogHook, hookErr := logger_util.NewFileHook(fullPath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o666)
			if hookErr != nil {
				return hookErr
			}
			log.Hooks.Add(free5gcLogHook)
		}
	} else {
		return err
	}

	if fullPath, err := logger_util.CreateNfLogFile(logNfPath, "ausf.log"); err == nil {
		selfLogHook, hookErr := logger_util.NewFileHook(fullPath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o666)
		if hookErr != nil {
			return hookErr
		}
		log.Hooks.Add(selfLogHook)
	} else {
		return err
	}

	return nil
}

func SetLogLevel(level logrus.Level) {
	log.SetLevel(level)
}

func SetReportCaller(enable bool) {
	log.SetReportCaller(enable)
}
