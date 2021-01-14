package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/spf13/viper"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("Run failed: %v", err)
	} else {
		log.Println("Exited without error")
	}
}

type Config struct {
	ConnectionString string
	Urls []string
}

func run() error {
	viper.SetConfigType("hcl")
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	err = viper.Unmarshal(&config)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	conn, err := connectToDatabase(&config)
	if err != nil {
		return fmt.Errorf("failed create database connection: %w", err)
	}

	log.Printf("config: %+v", config)

	runPings(&config, conn)
	return nil
}

type Pinger struct {
	url       string
	lastError error
	duration *time.Duration
}

func (p *Pinger) Ping(ctx context.Context, wg *sync.WaitGroup) {

	p.lastError = nil
	p.duration = nil

	defer wg.Done()

	start := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, http.NoBody)
	if err != nil {
		p.lastError = fmt.Errorf("creating request failed: %w", err)
		return
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		p.lastError = err
		return
	}

	p.lastError = response.Body.Close()
	end := time.Now()

	dur := end.Sub(start)
	p.duration = &dur
}

type PingResult struct {
	hadError bool
	duration *time.Duration
	url string
	startedAt time.Time
}

type PingManager struct {
	pingers []*Pinger
}

func (m *PingManager) runOnce(ctx context.Context) []PingResult {
	startingAt := time.Now()

	var wg sync.WaitGroup
	wg.Add(len(m.pingers))

	for _, pinger := range m.pingers {
		go pinger.Ping(ctx, &wg)
	}

	wg.Wait()

	results := make([]PingResult, len(m.pingers))

	for i, pinger := range m.pingers {
		results[i] = PingResult{
			hadError: pinger.lastError != nil,
			duration: pinger.duration,
			url: pinger.url,
			startedAt: startingAt,
		}
	}

	return results
}

func NewPingManager(config *Config) *PingManager {
	pingers := make([]*Pinger, len(config.Urls))

	for index, url := range config.Urls {
		pingers[index] = &Pinger {
			url: url,
		}
	}

	return &PingManager{
		pingers: pingers,
	}
}

func runPings(config *Config, conn *pgx.Conn) {

	log.Println("Everything is ready, running pings...")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<- c
		cancel()
	}()

	pingManager := NewPingManager(config)

	for cancelled := ctx.Err(); cancelled == nil; cancelled = ctx.Err() {
		runRound(ctx, conn, pingManager)
	}
}

func runRound(ctx context.Context, conn *pgx.Conn, pingManager *PingManager) {

	timeout, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	results := pingManager.runOnce(timeout)

	for _, result := range results {
		rows, err := conn.Query(ctx, `insert into "results"("time", "duration", "url", "had_error") values ($1, $2, $3, $4);`, result.startedAt, result.duration, result.url, result.hadError)
		if err != nil {
			log.Printf("failed to insert results: %v", err)
		} else {
			rows.Close()
		}
	}

	log.Println("Finished round and wrote to database")

	<- timeout.Done()
}