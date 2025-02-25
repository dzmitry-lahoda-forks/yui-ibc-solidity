package client

import (
	"context"
	"fmt"
	"time"

	"github.com/avast/retry-go"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type ETHClient struct {
	*ethclient.Client
	rpcClient *rpc.Client
	option    option
}

type Option func(*option)

type option struct {
	retryOpts []retry.Option
}

func DefaultOption() *option {
	return &option{
		retryOpts: []retry.Option{
			retry.Delay(1 * time.Second),
			retry.Attempts(10),
		},
	}
}

func WithRetryOption(rops ...retry.Option) Option {
	return func(opt *option) {
		opt.retryOpts = rops
	}
}

func NewETHClient(endpoint string, opts ...Option) (*ETHClient, error) {
	rpcClient, err := rpc.DialHTTP(endpoint)
	if err != nil {
		return nil, err
	}
	opt := DefaultOption()
	for _, o := range opts {
		o(opt)
	}
	return &ETHClient{
		rpcClient: rpcClient,
		Client:    ethclient.NewClient(rpcClient),
		option:    *opt,
	}, nil
}

func (cl *ETHClient) GetTransactionReceipt(ctx context.Context, txHash common.Hash) (*gethtypes.Receipt, bool, error) {
	var r *Receipt
	if err := cl.rpcClient.CallContext(ctx, &r, "eth_getTransactionReceipt", txHash); err != nil {
		return r.GetGethReceipt(), true, err
	}
	if r == nil {
		return nil, true, ethereum.NotFound
	} else if r.Status == gethtypes.ReceiptStatusSuccessful {
		return r.GetGethReceipt(), false, nil
	} else if r.HasRevertReason() {
		reason, err := r.GetRevertReason()
		return r.GetGethReceipt(), false, fmt.Errorf("revert-reason=%v parse-err=%v", reason, err)
	} else {
		return r.GetGethReceipt(), false, fmt.Errorf("failed to execute a transaction: %v", r)
	}
}

func (cl *ETHClient) WaitForReceiptAndGet(ctx context.Context, tx *gethtypes.Transaction) (*gethtypes.Receipt, error) {
	var receipt *gethtypes.Receipt
	err := retry.Do(
		func() error {
			rc, recoverable, err := cl.GetTransactionReceipt(ctx, tx.Hash())
			if err != nil {
				if recoverable {
					return err
				} else {
					return retry.Unrecoverable(err)
				}
			}
			receipt = rc
			return nil
		},
		cl.option.retryOpts...,
	)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}
