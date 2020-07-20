package main

import (
	"fmt"
	"bytes"
	"errors"
	"encoding/hex"
	"net/http"
	"encoding/json"
	"strconv"

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
// This is the go stub program.  It is compiled into all the CBOR 
// marshalling and structs from Lotus.  However, all it does is listen 
// on 80 and respond with well-formed unsigned messagess, which
// can be signed in rust/js.
//

func main() {
	http.HandleFunc("/paymentchannel/create", PaymentChannelCreate)
	http.ListenAndServe(":8080", nil)
}

//
//
// HTTP helpers
//
//
func failResponse(w http.ResponseWriter, responseStatus int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(responseStatus)
	errMsgMap := map[string]string{"Error": message}
	jsonBodyStr,_ := json.Marshal(errMsgMap)
	fmt.Fprintf(w, "%s", jsonBodyStr)
}

//
// Dispatcher for all `/paymentchannel/create` actions
//
func PaymentChannelDispatch(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err!=nil {
		failResponse(w, http.StatusBadRequest, "Could not parse your input")
		return
	}
	vals := r.Form
	action := vals.Get("action")
	if action=="message" {
		paymentChannelCreate(vals, w, r)
	} else if action=="decodereceipt") {
		paymentChannelDecodeReceipt(vals, w, r)
	} else {
		failResponse(w, http.StatusBadRequest, "Unrecognized 'action'")
		return
	}
}

//
// `/paymentchannel/create` endpoint
//
//	POST /paymentchannel/create?action=message&
//		to=t1ymsm3e4tavw4dsc6raxk6dioxi4coio4sp37lzq&
//		from=t1ymsm3e4tavw4dsc6raxk6dioxi4coio4sp37lzq&
//		amount=30
//	Response:
//		200 OK with body {"Version":0,.......}
//
//	or else faile with non-2XX response code
//
func paymentChannelCreate(url.Values vals, w http.ResponseWriter, r *http.Request) {
	toStr := vals.Get("to")
	fromStr := vals.Get("from")
	amountStr := vals.Get("amount")
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err!=nil {
		failResponse(w, http.StatusBadRequest, "'amount' not numeric")
		return
	}
	if (toStr=="") || (fromStr=="") {
		failResponse(w, http.StatusBadRequest, "'to' and 'from' invalid")
		return
	}

	msg, err := generateMessageForSigning(toStr, fromStr, amount)
	if err != nil {
		fmt.Errorf("generateMessageForSigning: faled");
	}


	toHexEncoded := hex.EncodeToString(msg.To.Bytes())
	fromHexEncoded := hex.EncodeToString(msg.From.Bytes())
	valueBytes,_ := msg.Value.Bytes()
	valueHexEncoded := hex.EncodeToString(valueBytes)
	gasPriceBytes,_ := msg.GasPrice.Bytes()
	gasPriceHexEncoded := hex.EncodeToString(gasPriceBytes)
	paramsHexEncoded := hex.EncodeToString(msg.Params)
	response := map[string]string{	"Version": strconv.FormatInt(msg.Version,10),
		"To":toHexEncoded, "From": fromHexEncoded,
		"Nonce": strconv.FormatUint(msg.Nonce,10), "Value":valueHexEncoded,
		"GasPrice":gasPriceHexEncoded, "GasLimit":strconv.FormatInt(msg.GasLimit,10),
		"Method":msg.Method.String() , "Params":paramsHexEncoded}
	responseJsonStr,_ := json.Marshal(response)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", responseJsonStr)
}

func generateMessageForSigning(toBase32Str string, fromBase32Str string, amountBase10 int64) (*Message, error) {
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
	amt := BigInt{Int: big.NewInt(amountBase10)}


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
func PaymentChannelPoll() {
	// It may make  more sense to do this call against a chain indexer instance
	// that has the whole history of the chain.
}

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
func paymentChannelDecodeReceipt(url.Values vals, w http.ResponseWriter, r *http.Request) {
	// TODO
}

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

