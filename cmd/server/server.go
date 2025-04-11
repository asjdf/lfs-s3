package server

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/asjdf/lfs-s3/cmd/server/modList"
	"github.com/juanjiTech/jframe/conf"
	"github.com/juanjiTech/jframe/core/kernel"
	"github.com/juanjiTech/jframe/core/logx"
	"github.com/juanjiTech/jframe/pkg/ip"
	"github.com/soheilhy/cmux"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	configPath string
	StartCmd   = &cobra.Command{
		Use:   "server",
		Short: "Start server -c ./config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			logx.PreInit()

			conf.LoadConfig(configPath)
			logx.Init(zapcore.DebugLevel)

			conn, err := net.Listen("tcp", fmt.Sprintf(":%s", conf.Get().Port))
			if err != nil {
				zap.S().Errorw("failed to listen", "error", err)
				return err
			}
			tcpMux := cmux.New(conn)
			zap.S().Infow("start listening", "port", conf.Get().Port)

			k := kernel.New(kernel.Config{})
			k.Map(&conn, &tcpMux)
			k.RegMod(modList.ModList...)

			k.Init()

			if err := k.StartModule(); err != nil {
				return err
			}

			k.Serve()

			go func() {
				_ = tcpMux.Serve()
			}()

			fmt.Println("Server run at:")
			fmt.Printf("-  Local:   http://localhost:%s\n", conf.Get().Port)
			for _, host := range ip.GetLocalHost() {
				fmt.Printf("-  Network: http://%s:%s\n", host, conf.Get().Port)
			}

			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			<-quit

			return k.Stop()
		},
	}
)

func init() {
	StartCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Start server with provided configuration file")
}
