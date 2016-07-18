package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
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
	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		log.Fatalf("Failed to load database: %v", err)
	}
	defer db.Close()

	f, err := os.Create("rrr.pprof")
	if err != nil {
		log.Panic(err)
	}
	defer f.Close()
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()

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
					pubKey, err := btcec.ParsePubKey(pushedData[1], btcec.S256())
					if err != nil {
						continue // TODO
					}
					rFound += 1

					fmt.Println(h, txN, txInN, sig.R.Text(16), pubKey.X.Text(16), pubKey.Y.Text(16))
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
