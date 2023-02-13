package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/decoder"
	"github.com/jacoblai/httprouter"
	"golang.org/x/sys/unix"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reuserhttp/cors"
	"reuserhttp/deny"
	primitive "reuserhttp/objectId"
	"reuserhttp/resultor"
	"sync"
	"syscall"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
	timeLocation, _ := time.LoadLocation("Asia/Shanghai") //使用时区码
	time.Local = timeLocation
	setLimit()
}

func main() {
	var (
		addr = flag.String("l", ":7009", "绑定Host地址")
	)
	flag.Parse()
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Println(err)
		return
	}

	queue := sync.Map{}

	router := httprouter.New()
	router.POST("/", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer r.Body.Close()
		var obj map[string]interface{}
		dc := decoder.NewStreamDecoder(r.Body)
		dc.UseInt64()
		err := dc.Decode(&obj)
		if err != nil {
			resultor.RetErr(w, err)
			return
		}
		//防注入
		bts, _ := sonic.Marshal(&obj)
		if !deny.Injection(bts) {
			resultor.RetErr(w, "1002")
			return
		}
		id := primitive.NewObjectID().Hex()
		queue.Store(id, obj)
		resultor.RetOk(w, id, 1)
	})
	router.GET("/", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer r.Body.Close()
		id, err := primitive.ObjectIDFromHex(r.URL.Query().Get("id"))
		if err != nil {
			resultor.RetErr(w, "id err")
			return
		}
		item, ok := queue.LoadAndDelete(id.Hex())
		if !ok {
			resultor.RetErr(w, "wow")
			return
		}
		resultor.RetOk(w, item, 1)
	})

	srv := &http.Server{Handler: cors.CORS(router), ErrorLog: nil}
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var socketErr error
			err := c.Control(func(fd uintptr) {
				socketErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
			if err != nil {
				return err
			}
			return socketErr
		},
	}
	ln, err := lc.Listen(context.Background(), "tcp", *addr)
	if err != nil {
		panic(err)
	}
	if *addr == ":443" {
		go func() {
			if err := srv.ServeTLS(ln, dir+"/data/api.lzyhr.com.pem", dir+"/data/api.lzyhr.com.key"); err != nil {
				log.Println(err)
			}
		}()
		log.Println("server on https port", *addr)
	} else {
		go func() {
			if err := srv.Serve(ln); err != nil {
				log.Println(err)
			}
		}()
		log.Println("server on http port", *addr)
	}

	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	cleanup := make(chan bool)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range signalChan {
			ctx, _ := context.WithTimeout(context.Background(), 60*time.Second)
			go func() {
				_ = srv.Shutdown(ctx)
				cleanup <- true
			}()
			<-cleanup
			fmt.Println("safe exit")
			cleanupDone <- true
		}
	}()
	<-cleanupDone
}

func setLimit() {
	// Increase resources limitations
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatal(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatal(err)
	}
}
