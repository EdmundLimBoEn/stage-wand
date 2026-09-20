package main

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/ble"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/input"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/lan"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/mdns"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/server"
)

func main() {
	jsonStatus := flag.Bool("json-status", false, "emit JSON status updates for the desktop app")
	serve := flag.Bool("serve", false, "headless test server with pairing code 0000")
	selftest := flag.Bool("selftest", false, "inject a short movement, click, Esc, and scroll")
	diagnose := flag.Bool("diagnose", false, "print uinput and session probe, then exit")
	codeFlag := flag.String("code", "", "pairing code (default: random 4 digits, or 0000 with --serve)")
	dryRun := flag.Bool("dry-run", false, "log commands without injecting input")
	name := flag.String("name", "", "mDNS instance name (default: hostname)")
	flag.Parse()
	if os.Getenv("STAGEWAND_DRY_RUN") == "1" {
		*dryRun = true
	}

	if *diagnose {
		fmt.Print(input.Diagnose().Report())
		return
	}

	if *selftest {
		os.Exit(runSelfTest(*dryRun))
	}

	code := *codeFlag
	if *serve && code == "" {
		code = "0000"
	}
	if code == "" {
		code = randomCode()
	}

	injector, injectNote := openInjector(*serve || *dryRun)
	defer injector.Close()

	session := server.NewSession(code)
	cfg := server.DefaultConfig()
	onCommand := func(command protocol.Command) {
		if *serve {
			data, err := protocol.EncodeCommand(command)
			if err == nil {
				fmt.Printf("COMMAND %s\n", data)
				os.Stdout.Sync()
			}
		}
		if err := injector.Apply(command); err != nil {
			fmt.Fprintf(os.Stderr, "input: %v\n", err)
		}
	}
	srv := server.New(session, cfg, onCommand)
	port, err := srv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
	if *serve {
		fmt.Printf("PORT %d\n", port)
		os.Stdout.Sync()
	}

	advert, mdnsErr := mdns.Advertise(*name, port)
	if advert != nil {
		defer advert.Close()
	}

	bleName := *name
	if bleName == "" {
		bleName, _ = os.Hostname()
	}
	var bleDev ble.Peripheral = ble.Unavailable(fmt.Errorf("--serve"))
	if !*serve {
		bleDev = ble.Start(ble.Hooks{
			Control:   session,
			OnCommand: onCommand,
		}, bleName)
		defer bleDev.Close()
	}

	if *serve {
		waitSignal()
		srv.Close()
		return
	}

	ready, readyNote := injector.Ready()
	if injectNote != "" {
		readyNote = injectNote
	}
	ip := lan.IPv4()
	mdnsNote := "advertised _stagewand._tcp"
	if mdnsErr != nil {
		mdnsNote = "unavailable (" + mdnsErr.Error() + "); use manual host:port"
	}
	var outputMu sync.Mutex
	var last string
	redraw := func() {
		outputMu.Lock()
		defer outputMu.Unlock()
		var snap string
		if *jsonStatus {
			data, _ := json.Marshal(desktopStatus(session, ip, port, ready, readyNote, mdnsNote, bleDev.Note()))
			snap = string(data) + "\n"
		} else {
			snap = snapshot(session, ip, port, readyNote, mdnsNote, bleDev.Note())
		}
		if snap != last {
			last = snap
			fmt.Print(snap)
		}
	}
	redraw()
	go stdinKick(session, srv, bleDev, redraw)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			redraw()
		}
	}()
	waitSignal()
	srv.Close()
}

func openInjector(logOnly bool) (input.Injector, string) {
	probe := input.Diagnose()
	if logOnly {
		return input.Logging{}, "dry-run (commands logged, not injected); session " + string(probe.Session)
	}
	injector, err := input.OpenOrError()
	if err != nil {
		fmt.Fprintf(os.Stderr, "input injection unavailable: %v\n", err)
		fmt.Fprintf(os.Stderr, "continuing in dry-run. See README Linux host for Arch uinput setup.\n")
		return input.Logging{}, "dry-run: " + probe.Hint
	}
	_, note := injector.Ready()
	return injector, note
}

func runSelfTest(dryRun bool) int {
	probe := input.Diagnose()
	fmt.Print(probe.Report())
	if dryRun {
		injector := input.Logging{}
		if err := applySelfTest(injector); err != nil {
			fmt.Printf("SELFTEST FAIL: %v\n", err)
			return 1
		}
		fmt.Println("SELFTEST PASS: dry-run posted pointer movement, left click, Esc, and scroll.")
		return 0
	}
	injector, err := input.OpenOrError()
	if err != nil {
		fmt.Printf("SELFTEST FAIL: %s\n", err)
		return 1
	}
	defer injector.Close()
	ok, detail := injector.Ready()
	if !ok {
		fmt.Printf("SELFTEST FAIL: %s\n", detail)
		return 1
	}
	fmt.Printf("SELFTEST injector: %s\n", detail)
	if err := applySelfTest(injector); err != nil {
		fmt.Printf("SELFTEST FAIL: %v\n", err)
		return 1
	}
	fmt.Println("SELFTEST PASS: Posted pointer movement, left click, Esc, and scroll.")
	return 0
}

func applySelfTest(injector input.Injector) error {
	commands := []protocol.Command{
		protocol.Move{Dx: 100, Dy: 0}, protocol.Move{Dx: -100, Dy: 0},
		protocol.Click{Button: protocol.ButtonLeft}, protocol.KeyPress{Key: protocol.KeyEsc},
		protocol.Scroll{Dx: 0, Dy: 3},
	}
	for i, command := range commands {
		if err := injector.Apply(command); err != nil {
			return fmt.Errorf("%T: %w", command, err)
		}
		if i == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}

func randomCode() string {
	var n uint32
	if err := binary.Read(rand.Reader, binary.LittleEndian, &n); err != nil {
		return "0000"
	}
	return fmt.Sprintf("%04d", n%10000)
}

func waitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}

func stdinKick(session *server.Session, srv *server.Server, bleDev ble.Peripheral, redraw func()) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if line == "k" || line == "kick" {
			session.SetCode(randomCode())
			srv.Kick()
			bleDev.Kick()
			redraw()
		}
	}
}

func printUI(session *server.Session, ip string, port int, inputNote, mdnsNote, bleNote string) {
	fmt.Print(snapshot(session, ip, port, inputNote, mdnsNote, bleNote))
}

func snapshot(session *server.Session, ip string, port int, inputNote, mdnsNote, bleNote string) string {
	addr := fmt.Sprintf("port %d", port)
	if ip != "" {
		addr = fmt.Sprintf("%s:%d", ip, port)
	}
	peer := session.Peer()
	if peer == "" {
		peer = "waiting"
	}
	return fmt.Sprintf("\nStage Wand host\nPairing code:  %s\nListen:        %s\nInput:         %s\nPeer:          %s\nmDNS:          %s\nBluetooth:     %s\nType k then Enter to kick. Ctrl+C to quit.\n",
		session.Code(), addr, inputNote, peer, mdnsNote, bleNote)
}

// DesktopStatus is the line-delimited status contract consumed by the tray app.
type DesktopStatus struct {
	Type       string `json:"type"`
	Code       string `json:"code"`
	Address    string `json:"address"`
	Peer       string `json:"peer"`
	InputReady bool   `json:"inputReady"`
	Input      string `json:"input"`
	MDNS       string `json:"mdns"`
	Bluetooth  string `json:"bluetooth"`
}

func desktopStatus(session *server.Session, ip string, port int, ready bool, inputNote, mdnsNote, bleNote string) DesktopStatus {
	return DesktopStatus{Type: "status", Code: session.Code(), Address: fmt.Sprintf("%s:%d", ip, port), Peer: session.Peer(), InputReady: ready, Input: inputNote, MDNS: mdnsNote, Bluetooth: bleNote}
}
