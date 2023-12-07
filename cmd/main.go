package main

import (
	"github.com/heyvito/eswim"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var logger *zap.Logger
	var err error
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.DisableCaller = true
	logger, err = config.Build()
	if err != nil {
		panic(err)
	}

	opts := eswim.Options{
		IPv4MulticastAddress:   "224.0.0.244",
		MulticastPort:          1337,
		SWIMPort:               1338,
		InsecureDisableCrypto:  true,
		UseAdaptivePingTimeout: false,
		LogHandler:             logger,
	}
	srv, err := eswim.NewServer(&opts)
	if err != nil {
		panic(err)
	}
	srv.Start()
	sigChan := make(chan os.Signal, 3)
	go func() {
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	}()

	<-sigChan
	srv.Shutdown()
}
