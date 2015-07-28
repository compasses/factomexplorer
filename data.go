package main

import (
	"errors"
	"fmt"
	"github.com/FactomProject/FactomCode/common"
	"github.com/FactomProject/factoid/block"
	"github.com/FactomProject/factom"
	"log"
	"time"
)

var DBlocks map[string]DBlock
var DBlockKeyMRsBySequence map[int]string
var Blocks map[string]Block

type DataStatusStruct struct {
	DBlockHeight      int
	FullySynchronized bool
	LastKnownBlock    string
}

var DataStatus DataStatusStruct

func init() {
	DBlocks = map[string]DBlock{}
	DBlockKeyMRsBySequence = map[int]string{}
	Blocks = map[string]Block{}

	DataStatus.LastKnownBlock = "0000000000000000000000000000000000000000000000000000000000000000"
}

type DBlock struct {
	factom.DBlock

	BlockTimeStr string
	KeyMR        string

	Blocks int

	AdminEntries       int
	EntryCreditEntries int
	FactoidEntries     int
	EntryEntries       int

	AdminBlock       Block
	FactoidBlock     Block
	EntryCreditBlock Block
}

type Block struct {
	ChainID       string
	Hash          string
	PrevBlockHash string
	Timestamp     string

	EntryCount int

	EntryList []Entry

	IsAdminBlock       bool
	IsFactoidBlock     bool
	IsEntryCreditBlock bool
	IsEntryBlock       bool
}

type Entry struct {
	//Marshallable blocks
	BinaryString string
	Timestamp    string
	Hash         string
}

func GetDBlockFromFactom(keyMR string) (DBlock, error) {
	var answer DBlock

	body, err := factom.GetDBlock(keyMR)
	if err != nil {
		return answer, err
	}

	answer = DBlock{DBlock: *body}
	answer.BlockTimeStr = TimestampToString(body.Header.TimeStamp)
	answer.KeyMR = keyMR

	return answer, nil
}

func TimestampToString(timestamp uint64) string {
	blockTime := time.Unix(int64(timestamp), 0)
	return blockTime.Format("2006-01-02 15:04:05")
}

func Synchronize() error {
	head, err := factom.GetDBlockHead()
	if err != nil {
		return err
	}
	previousKeyMR := head.KeyMR
	for {
		block, exists := DBlocks[previousKeyMR]
		if exists {
			if DataStatus.FullySynchronized == true {
				break
			} else {
				previousKeyMR = block.Header.PrevBlockKeyMR
				continue
			}
		}
		body, err := GetDBlockFromFactom(previousKeyMR)
		if err != nil {
			return err
		}
		str, err := EncodeJSONString(body)
		if err != nil {
			return err
		}
		log.Printf("%v", str)

		for _, v := range body.EntryBlockList {
			fetchedBlock, err := FetchBlock(v.ChainID, v.KeyMR, body.BlockTimeStr)
			if err != nil {
				return err
			}
			switch v.ChainID {
			case "000000000000000000000000000000000000000000000000000000000000000a":
				body.AdminEntries += fetchedBlock.EntryCount
				body.AdminBlock = fetchedBlock
				break
			case "000000000000000000000000000000000000000000000000000000000000000c":
				body.EntryCreditEntries += fetchedBlock.EntryCount
				body.EntryCreditBlock = fetchedBlock
				break
			case "000000000000000000000000000000000000000000000000000000000000000f":
				body.FactoidEntries += fetchedBlock.EntryCount
				body.FactoidBlock = fetchedBlock
				break
			default:
				body.EntryEntries += fetchedBlock.EntryCount
				break
			}
		}
		body.EntryBlockList = body.EntryBlockList[3:]

		DBlocks[previousKeyMR] = body
		DBlockKeyMRsBySequence[body.Header.SequenceNumber] = previousKeyMR
		if DataStatus.DBlockHeight < body.Header.SequenceNumber {
			DataStatus.DBlockHeight = body.Header.SequenceNumber
		}
		previousKeyMR = body.Header.PrevBlockKeyMR
		if previousKeyMR == "0000000000000000000000000000000000000000000000000000000000000000" {
			DataStatus.FullySynchronized = true
			break
		}

	}
	return nil
}

func FetchBlock(chainID, hash, blockTime string) (Block, error) {
	var block Block

	raw, err := factom.GetRaw(hash)
	if err != nil {
		return block, err
	}
	switch chainID {
	case "000000000000000000000000000000000000000000000000000000000000000a":
		block, err = ParseAdminBlock(chainID, hash, raw, blockTime)
		if err != nil {
			return block, err
		}
		break
	case "000000000000000000000000000000000000000000000000000000000000000c":
		block, err = ParseEntryCreditBlock(chainID, hash, raw, blockTime)
		if err != nil {
			return block, err
		}
		break
	case "000000000000000000000000000000000000000000000000000000000000000f":
		block, err = ParseFactoidBlock(chainID, hash, raw, blockTime)
		if err != nil {
			return block, err
		}
		break
	default:
		block, err = ParseEntryBlock(chainID, hash, raw, blockTime)
		if err != nil {
			return block, err
		}
		break
	}
	Blocks[hash] = block

	return block, nil
}

func ParseEntryCreditBlock(chainID, hash string, rawBlock []byte, blockTime string) (Block, error) {
	var answer Block

	ecBlock := common.NewECBlock()
	_, err := ecBlock.UnmarshalBinaryData(rawBlock)
	if err != nil {
		return answer, err
	}

	answer.ChainID = chainID
	answer.Hash = hash
	answer.EntryCount = len(ecBlock.Body.Entries)
	answer.PrevBlockHash = fmt.Sprintf("%x", ecBlock.Header.PrevFullHash.GetBytes())
	answer.EntryList = make([]Entry, answer.EntryCount)

	for i, v := range ecBlock.Body.Entries {
		marshalled, err := v.MarshalBinary()
		if err != nil {
			return answer, err
		}
		answer.EntryList[i].BinaryString = fmt.Sprintf("%x", marshalled)
		answer.EntryList[i].Timestamp = blockTime
	}

	answer.IsEntryCreditBlock = true

	return answer, nil
}

func ParseFactoidBlock(chainID, hash string, rawBlock []byte, blockTime string) (Block, error) {
	var answer Block

	fBlock := new(block.FBlock)
	_, err := fBlock.UnmarshalBinaryData(rawBlock)
	if err != nil {
		return answer, nil
	}

	answer.ChainID = chainID
	answer.Hash = hash
	answer.PrevBlockHash = fmt.Sprintf("%x", fBlock.PrevKeyMR.Bytes())
	transactions := fBlock.GetTransactions()
	answer.EntryCount = len(transactions)
	answer.EntryList = make([]Entry, answer.EntryCount)
	for i, v := range transactions {
		answer.EntryList[i].BinaryString = v.String()
		answer.EntryList[i].Timestamp = TimestampToString(v.GetMilliTimestamp() / 1000)
		answer.EntryList[i].Hash = v.GetHash().String()
	}

	answer.IsFactoidBlock = true

	return answer, nil
}

func ParseEntryBlock(chainID, hash string, rawBlock []byte, blockTime string) (Block, error) {
	var answer Block

	eBlock := common.NewEBlock()
	_, err := eBlock.UnmarshalBinaryData(rawBlock)
	if err != nil {
		return answer, err
	}

	answer.ChainID = chainID
	answer.Hash = hash

	answer.PrevBlockHash = eBlock.Header.PrevKeyMR.ByteString()

	answer.EntryCount = len(eBlock.Body.EBEntries)
	answer.EntryList = make([]Entry, answer.EntryCount)

	for i, v := range eBlock.Body.EBEntries {
		answer.EntryList[i].BinaryString = v.ByteString()
		answer.EntryList[i].Timestamp = blockTime
		answer.EntryList[i].Hash = v.ByteString()
	}

	answer.IsEntryBlock = true

	return answer, nil
}

func ParseAdminBlock(chainID, hash string, rawBlock []byte, blockTime string) (Block, error) {
	var answer Block

	aBlock := new(common.AdminBlock)
	_, err := aBlock.UnmarshalBinaryData(rawBlock)
	if err != nil {
		return answer, err
	}

	answer.ChainID = chainID
	answer.Hash = hash
	answer.EntryCount = len(aBlock.ABEntries)
	answer.PrevBlockHash = fmt.Sprintf("%x", aBlock.Header.PrevFullHash.GetBytes())
	answer.EntryList = make([]Entry, answer.EntryCount)
	for i, v := range aBlock.ABEntries {
		marshalled, err := v.MarshalBinary()
		if err != nil {
			return answer, err
		}
		answer.EntryList[i].BinaryString = fmt.Sprintf("%x", marshalled)
		answer.EntryList[i].Timestamp = blockTime
	}

	answer.IsAdminBlock = true

	return answer, nil
}

func GetBlock(keyMR string) (Block, error) {
	block, ok := Blocks[keyMR]
	if ok == false {
		return block, fmt.Errorf("Block %v not found", keyMR)
	}
	return block, nil
}

func GetBlockHeight() int {
	return DataStatus.DBlockHeight
}

func GetDBlocks(start, max int) []DBlock {
	answer := []DBlock{}
	for i := start; i < max; i++ {
		keyMR := DBlockKeyMRsBySequence[i]
		if keyMR == "" {
			continue
		}
		answer = append(answer, DBlocks[keyMR])
	}
	return answer
}

func GetDBlock(keyMR string) (DBlock, error) {
	block, ok := DBlocks[keyMR]
	if ok != true {
		return block, errors.New("DBlock not found")
	}
	return block, nil
}

type DBInfo struct {
	BTCTxHash string
}

func GetDBInfo(keyMR string) (DBInfo, error) {
	//TODO: gather DBInfo
	return DBInfo{}, nil
}

type EBlock struct {
	factom.EBlock
}
