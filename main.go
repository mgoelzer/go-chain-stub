package main

import (
	"fmt"
	"bytes"
	"errors"
	"encoding/hex"

//	types "github.com/filecoin-project/lotus/chain/types"
	"math/big"
	big2 "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/abi"


	"github.com/filecoin-project/go-address"


	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	paych "github.com/filecoin-project/specs-actors/actors/builtin/paych"
	cbg "github.com/whyrusleeping/cbor-gen"
	"github.com/ipfs/go-cid"
)

//
// Some types and functions pilfered from Lotus
//

type BigInt = big2.Int

func actorsSerializeParams(i cbg.CBORMarshaler) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := i.MarshalCBOR(buf); err != nil {
		return nil, errors.New("failed to encode parameter")
	}
	return buf.Bytes(), nil
}

type Message struct {
	Version int64

	To   address.Address
	From address.Address

	Nonce uint64

	Value BigInt

	GasPrice BigInt
	GasLimit int64

	Method abi.MethodNum
	Params []byte
}

//
// This is the go stub program.  It is compiled into all the CBOR marshalling and structs from Lotus.
// However, all it does is listen on 443 and respond with well-formed unsigned messagess, which
// can be signed in rust/js.
//

func main() {
	// A dummy main() to illustrate the usage
	toBase32Str := "t1ymsm3e4tavw4dsc6raxk6dioxi4coio4sp37lzq"
	fromBase32Str := "t1ymsm3e4tavw4dsc6raxk6dioxi4coio4sp37lzq"
	amountBase10 := 30
	msg, err := generateMessageForSigning(toBase32Str, fromBase32Str, amountBase10)
	if err != nil {
		fmt.Errorf("generateMessageForSigning: faled");
	}

	fmt.Printf("msg.To (string) = '%s'\n", msg.To.String())
	toHexEncoded := hex.EncodeToString(msg.To.Bytes())
	fmt.Printf("msg.To (bytes) = 0x %s\n", toHexEncoded)

	fmt.Printf("msg.From (string) = '%s'\n", msg.From.String())
	fromHexEncoded := hex.EncodeToString(msg.From.Bytes())
	fmt.Printf("msg.From (bytes) = 0x %s\n", fromHexEncoded)

	paramsHexEncoded := hex.EncodeToString(msg.Params)
	fmt.Printf("msg.Params (bytes) = 0x %s\n", paramsHexEncoded)

}

//
// `/paymentchannel/create/` endpoint
//
func generateMessageForSigning(toBase32Str string, fromBase32Str string, amountBase10 int) (*Message, error) {
	// From and To addresses  are just whatever user typed on CLI
	from, err := address.NewFromString(toBase32Str)
	if err != nil {
		fmt.Errorf("failed to parse from address: %s", err)
		return nil, err
	}

	to, err := address.NewFromString(fromBase32Str)
	if err != nil {
		fmt.Errorf("failed to parse to address: %s", err)
		return nil, err
	}

	//30 is just an Int with whatever number the user enters on CLI.
	amt := BigInt{Int: big.NewInt(30)}


	///////////////////////////////////////////////////////////////
	//
	// Code from lotus/paychmgr/simple.go
	//
	///////////////////////////////////////////////////////////////

	params, aerr := actorsSerializeParams(&paych.ConstructorParams{From: from, To: to})
	if aerr != nil {
		fmt.Errorf("Failed at actorsSerializeParams(from,to)")
		return nil,aerr
	}

	enc, aerr := actorsSerializeParams(&init_.ExecParams{
		CodeCID:           builtin.PaymentChannelActorCodeID,
		ConstructorParams: params,
	})
	if aerr != nil {
		fmt.Errorf("Failed at actorsSerializeParams(init_")
		return nil,aerr
	}

	msg := &Message{
		To:       builtin.InitActorAddr,
		From:     from,
		Value:    amt,
		Method:   builtin.MethodsInit.Exec,
		Params:   enc,
		GasLimit: 1000000,
		GasPrice: BigInt{Int: big.NewInt(0)},
	}

	//
	// All that's missing from above is Nonce and Signature:
	//	Nonce - this can be determined by calling API method 
	//		MpoolGetNonce(address.Address) for the "from" address
	//	Signature - but we don't need this in go, the Zondax library can 
	//		already do that
	//

	return msg,nil
}

//
// `/paymentchannel/create?action=poll&mcid=<cid>&message_confidence=5`
//
func pollForReceipt(mcid cid.Cid, MessageConfidence uint64) (int64, string, uint64, error) {
	// The purpose of this API call is to let client poll until submitted message's Receipt
	// can be retruned, which is MessageConfidence epochs after the message receipt is found.
	//
	// TODO:
	// 1.  Write code to scan the messages appearing on new blocks until we see
	// a receipt for our message message (mcid)
	// 2.  Decode the receipt into the tuple {exitCode, ReturnString, gasUsed}
	// 3.  If (height of mcid)+MessageConfidence<=current tipset height, then return immediately
	// with {0,"",0,ERROR_NOT_READY}.  Else, return immediately with {exitCode, ReturnString, gasUsed, nill}.

	// Return dummy data for now
	return 0,"TzLsta5iEBaiWoZjvfK+9hwIDAkKk9UAUPnmUwCtT5EuqYVofXWfpzJw==",1000,nil
}

//
// `/paymentchannel/create?action=decode_receipt?exit_code=0&returnbytes=%0f%20%10%00%10...&gasUsed=10000
//
func decodeReceipt(exitCode int64, returnBytes []byte, gasUsed uint64) (*address.Address,error) {
	// This function decodes the receipt and returns the payment channel address.
	var decodedReturn init_.ExecReturn
	err := decodedReturn.UnmarshalCBOR(bytes.NewReader(returnBytes))
	if err != nil {
		fmt.Errorf("err=%v",err)
		return nil,err
	}
	paychaddr := decodedReturn.RobustAddress
	return &paychaddr,nil
}

