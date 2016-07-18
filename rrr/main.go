package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

var (
	btcdHomeDir           = btcutil.AppDataDir("btcd", false)
	dataDir               = filepath.Join(btcdHomeDir, "data", "mainnet")
	heightIndexBucketName = []byte("heightidx")
	byteOrder             = binary.LittleEndian
)

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (database.DB, error) {
	// The database name is based on the database type.
	dbName := "blocks_ffldb"
	dbPath := filepath.Join(dataDir, dbName)

	log.Printf("Loading block database from '%s'", dbPath)
	db, err := database.Open("ffldb", dbPath, wire.MainNet)
	if err != nil {
		// Return the error if it's not because the database doesn't
		// exist.
		if dbErr, ok := err.(database.Error); !ok || dbErr.ErrorCode !=
			database.ErrDbDoesNotExist {

			return nil, err
		}

		// Create the db if it does not exist.
		err = os.MkdirAll(dataDir, 0700)
		if err != nil {
			return nil, err
		}
		db, err = database.Create("ffldb", dbPath, wire.MainNet)
		if err != nil {
			return nil, err
		}
	}

	log.Println("Block database loaded")
	return db, nil
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		log.Fatalf("Failed to load database: %v", err)
	}
	defer db.Close()

	lowestBlock, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Panic(err)
	}

	if err := db.View(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		heightIndex := meta.Bucket(heightIndexBucketName)

		doneBlocks, rFound := 0, 0
		if err := heightIndex.ForEach(func(k, v []byte) error {
			var hash wire.ShaHash
			copy(hash[:], v)
			h := byteOrder.Uint32(k)

			if h < uint32(lowestBlock) {
				return nil
			}

			blockBytes, err := dbTx.FetchBlock(&hash)
			if err != nil {
				return err
			}
			block := &wire.MsgBlock{}
			block.Deserialize(bytes.NewBuffer(blockBytes))
			for txN, tx := range block.Transactions {
				if blockchain.IsCoinBaseTx(tx) {
					continue
				}
				for txInN, txIn := range tx.TxIn {
					if !txscript.IsPushOnlyScript(txIn.SignatureScript) {
						continue // TODO: multisig
					}
					pushedData, err := txscript.PushedData(txIn.SignatureScript)
					if err != nil {
						disasm, _ := txscript.DisasmString(txIn.SignatureScript)
						log.Println(h, txN, txInN, disasm)
						continue
					}
					if len(pushedData) != 2 {
						continue // TODO
					}
					sig, err := btcec.ParseSignature(pushedData[0], btcec.S256())
					if err != nil {
						continue // TODO
					}
					x, ybit, err := ParsePubKey(pushedData[1])
					if err != nil {
						continue // TODO
					}
					rFound += 1

					fmt.Println(h, txN, txInN, sig.R.Text(16), x.Text(16), ybit)
				}
			}

			doneBlocks += 1
			if doneBlocks%10000 == 0 {
				log.Println(doneBlocks, rFound)
			}

			return nil
		}); err != nil {
			return err
		}

		log.Println(doneBlocks, rFound)

		return nil
	}); err != nil {
		log.Fatal(err)
	}
}

const (
	pubkeyCompressed   byte = 0x2 // y_bit + x coord
	pubkeyUncompressed byte = 0x4 // x coord + y coord
	pubkeyHybrid       byte = 0x6 // y_bit + x coord + y coord
)

func ParsePubKey(pubKeyStr []byte) (x *big.Int, ybit int, err error) {
	if len(pubKeyStr) == 0 {
		return nil, 0, errors.New("pubkey string is empty")
	}

	format := pubKeyStr[0]
	ybit = int(format & 0x1)
	format &= ^byte(0x1)

	switch len(pubKeyStr) {
	case btcec.PubKeyBytesLenUncompressed:
		if format != pubkeyUncompressed && format != pubkeyHybrid {
			return nil, 0, fmt.Errorf("invalid magic in pubkey str: "+
				"%d", pubKeyStr[0])
		}

		x = new(big.Int).SetBytes(pubKeyStr[1:33])
		if format == pubkeyUncompressed {
			y := new(big.Int).SetBytes(pubKeyStr[33:])
			ybit = int(y.Bit(0))
		}
	case btcec.PubKeyBytesLenCompressed:
		if format != pubkeyCompressed {
			return nil, 0, fmt.Errorf("invalid magic in compressed "+
				"pubkey string: %d", pubKeyStr[0])
		}
		x = new(big.Int).SetBytes(pubKeyStr[1:33])
	default: // wrong!
		return nil, 0, fmt.Errorf("invalid pub key length %d",
			len(pubKeyStr))
	}

	return
}
