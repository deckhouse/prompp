package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"os"
	"strings"
	"time"
)

func main() {
	headPath := os.Args[1]
	fmt.Println("loading head at", headPath)
	numberOfShards, err := getNumberOfShards(headPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	h, corrupted, numberOfSegments, err := head.Load(
		"test_head",
		0,
		headPath,
		nil,
		numberOfShards,
		10000,
		head.NoOpLastAppendedSegmentIDSetter{},
		prometheus.DefaultRegisterer,
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("head loaded")

	if corrupted {
		fmt.Println("head is corrupted")
	}

	fmt.Println("number of segments", numberOfSegments)

	_ = h
	time.Sleep(time.Minute)
}

func getNumberOfShards(headPath string) (numberOfShards uint16, err error) {
	dir, err := os.Open(headPath)
	if err != nil {
		return 0, err
	}
	defer dir.Close()

	files, err := dir.ReadDir(0)
	if err != nil {
		return 0, err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasPrefix(file.Name(), "shard_") && strings.HasSuffix(file.Name(), ".wal") {
			numberOfShards++
		}
	}

	return numberOfShards, nil
}
