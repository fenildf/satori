package g

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/leancloud/satori/agent/rpc"
	"github.com/leancloud/satori/common/cpool"
	"github.com/leancloud/satori/common/model"
)

//----------------------------

type TransferClient struct {
	cli  *rpc.RpcClient
	name string
}

func (this TransferClient) Name() string {
	return this.name
}

func (this TransferClient) Closed() bool {
	return this.cli == nil
}

func (this TransferClient) Close() error {
	this.cli = nil
	return nil
}

func (this TransferClient) Call(metrics interface{}) error {
	var resp model.TransferResponse
	err := this.cli.Call("Transfer.Update", metrics, &resp)
	if Config().Debug {
		log.Println("<=", &resp)
	}
	return err
}

func transferConnect(name string, p *cpool.ConnPool) (cpool.NConn, error) {
	connTimeout := time.Duration(p.ConnTimeout) * time.Millisecond
	cli := &rpc.RpcClient{
		RpcServer: p.Address,
		Timeout:   connTimeout,
	}

	return TransferClient{
		cli:  cli,
		name: name,
	}, nil
}

var (
	transferClients map[string]*cpool.ConnPool = map[string]*cpool.ConnPool{}

	metricsBufferLock *sync.RWMutex        = new(sync.RWMutex)
	metricsBuffer     []*model.MetricValue = make([]*model.MetricValue, 0, 5)
)

// -------------------------
func sendMetrics() {
	metricsBufferLock.Lock()
	if len(metricsBuffer) == 0 {
		metricsBufferLock.Unlock()
		return
	}
	send := metricsBuffer
	metricsBuffer = make([]*model.MetricValue, 0, 5)
	metricsBufferLock.Unlock()

	addrs := Config().Transfer.Addrs

	for c := 0; c < 3; c++ {
		for _, i := range rand.Perm(len(addrs)) {
			cli := transferClients[addrs[i]]
			err := cli.Call(send)
			if err != nil {
				log.Println("sendMetrics fail", addrs[i], err)
				continue
			}
			return
		}
	}
	log.Printf("%s\n", "No available transfer client to send metrics, metrics dropped!")
}

func SendToTransferProc() {
	rand.Seed(time.Now().UnixNano())
	cfg := Config().Transfer
	for _, addr := range cfg.Addrs {
		transferClients[addr] = cpool.NewConnPool(
			"transfer", addr, 5, 3, cfg.Timeout, cfg.Timeout, transferConnect,
		)
	}

	for {
		time.Sleep(5 * time.Second)
		go sendMetrics()
	}
}

func SendToTransfer(metrics []*model.MetricValue) {
	if len(metrics) == 0 {
		return
	}

	metrics = filterMetrics(metrics)

	if len(metrics) == 0 {
		return
	}

	debug := Config().Debug

	if debug {
		log.Printf("=> <Total=%d> %v\n", len(metrics), metrics[0])
	}

	metricsBufferLock.Lock()
	defer metricsBufferLock.Unlock()

	metricsBuffer = append(metricsBuffer, metrics...)
}
