package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"path"
	"runtime"

	"github.com/DaniilSokolyuk/go-pcap2socks/cfg"
	"github.com/DaniilSokolyuk/go-pcap2socks/core"
	"github.com/DaniilSokolyuk/go-pcap2socks/core/device"
	"github.com/DaniilSokolyuk/go-pcap2socks/core/option"
	"github.com/DaniilSokolyuk/go-pcap2socks/i18n"
	"github.com/DaniilSokolyuk/go-pcap2socks/proxy"
	"github.com/jackpal/gateway"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

//go:embed config.json
var configData string

func main() {
	// Setup logging - check SLOG_LEVEL env var
	logLevel := slog.LevelInfo // Default to debug
	if lvl := os.Getenv("SLOG_LEVEL"); lvl != "" {
		switch lvl {
		case "debug", "DEBUG":
			logLevel = slog.LevelDebug
		case "info", "INFO":
			logLevel = slog.LevelInfo
		case "warn", "WARN":
			logLevel = slog.LevelWarn
		case "error", "ERROR":
			logLevel = slog.LevelError
		}
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(handler))

	// Check for config command
	if len(os.Args) > 1 && os.Args[1] == "config" {
		openConfigInEditor()
		return
	}

	// get config file from first argument or use config.json
	var cfgFile string
	if len(os.Args) > 1 {
		cfgFile = os.Args[1]
	} else {
		executable, err := os.Executable()
		if err != nil {
			slog.Error("get executable error", slog.Any("err", err))
			return
		}

		cfgFile = path.Join(path.Dir(executable), "config.json")
	}

	cfgExists := cfg.Exists(cfgFile)
	if !cfgExists {
		slog.Info("Config file not found, creating a new one", "file", cfgFile)
		//path to near executable file
		err := os.WriteFile(cfgFile, []byte(configData), 0666)
		if err != nil {
			slog.Error("write config error", slog.Any("file", cfgFile), slog.Any("err", err))
			return
		}
	}

	config, err := cfg.Load(cfgFile)
	if err != nil {
		slog.Error("load config error", slog.Any("file", cfgFile), slog.Any("err", err))
		return
	}
	slog.Info("Config loaded", "file", cfgFile)

	// Initialize localizer with language from config
	localizer := i18n.NewLocalizer(i18n.Language(config.Language))
	msgs := localizer.GetMessages()

	if len(config.ExecuteOnStart) > 0 {
		slog.Info(msgs.ExecutingCommands, "cmd", config.ExecuteOnStart)

		var cmd *exec.Cmd
		if len(config.ExecuteOnStart) > 1 {
			cmd = exec.Command(config.ExecuteOnStart[0], config.ExecuteOnStart[1:]...)
		} else {
			cmd = exec.Command(config.ExecuteOnStart[0])
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		go func() {
			err := cmd.Start()
			if err != nil {
				slog.Error(msgs.ExecuteCommandError, slog.Any("err", err))
			}

			err = cmd.Wait()
			if err != nil {

			}
		}()
	}

	err = run(config, localizer)
	if err != nil {
		slog.Error("run error", slog.Any("err", err))
		return
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(msgs.HelloWorld))
	})
	log.Fatal(http.ListenAndServe(":8085", nil))
}

func run(cfg *cfg.Config, localizer *i18n.Localizer) error {
	msgs := localizer.GetMessages()

	// Find the interface first
	ifce := findInterface(cfg.PCAP.InterfaceGateway, localizer)
	slog.Info(msgs.UsingInterface, "interface", ifce.Name, "mac", ifce.HardwareAddr.String())

	// Parse network configuration
	netConfig, err := parseNetworkConfig(cfg.PCAP, ifce, localizer)
	if err != nil {
		return err
	}

	// Display network configuration
	displayNetworkConfig(netConfig, localizer)

	proxies := make(map[string]proxy.Proxy)
	for _, outbound := range cfg.Outbounds {
		var p proxy.Proxy
		switch {
		case outbound.Direct != nil:
			p = proxy.NewDirect()
		case outbound.Socks != nil:
			p, err = proxy.NewSocks5(outbound.Socks.Address, outbound.Socks.Username, outbound.Socks.Password)
			if err != nil {
				return fmt.Errorf("%s: %w", msgs.NewSocks5Error, err)
			}
		case outbound.Reject != nil:
			p = proxy.NewReject()
		case outbound.DNS != nil:
			p = proxy.NewDNS(cfg.DNS, ifce.Name)
		default:
			return fmt.Errorf("%s: %+v", msgs.InvalidOutbound, outbound)
		}

		proxies[outbound.Tag] = p
	}

	_defaultProxy = proxy.NewRouter(cfg.Routing.Rules, proxies)
	proxy.SetDialer(_defaultProxy)

	_defaultDevice, err = device.Open(cfg.Capture, ifce, netConfig, func() device.Stacker {
		return _defaultStack
	})
	if err != nil {
		return err
	}

	if _defaultStack, err = core.CreateStack(&core.Config{
		LinkEndpoint:     _defaultDevice,
		TransportHandler: &core.Tunnel{},
		MulticastGroups:  []net.IP{},
		Options:          []option.Option{},
	}); err != nil {
		slog.Error(msgs.CreateStackError, slog.Any("err", err))
	}

	return nil
}

var (
	// _defaultProxy holds the default proxy for the engine.
	_defaultProxy proxy.Proxy

	// _defaultDevice holds the default device for the engine.
	_defaultDevice device.Device

	// _defaultStack holds the default stack for the engine.
	_defaultStack *stack.Stack
)

func findInterface(cfgIfce string, localizer *i18n.Localizer) net.Interface {
	msgs := localizer.GetMessages()
	var targetIP net.IP
	if cfgIfce != "" {
		targetIP = net.ParseIP(cfgIfce)
		if targetIP == nil {
			panic(fmt.Errorf("%s: %s", msgs.ParseIPError, cfgIfce))
		}
	} else {
		var err error
		targetIP, err = gateway.DiscoverInterface()
		if err != nil {
			panic(fmt.Errorf("%s: %w", msgs.DiscoverInterfaceError, err))
		}
	}

	// Get a list of all interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip4 := ipnet.IP.To4()
			if ip4 != nil && bytes.Equal(ip4, targetIP.To4()) {
				return iface
			}
		}
	}

	panic(fmt.Errorf(msgs.InterfaceNotFound, targetIP))
}

func parseNetworkConfig(pcapCfg cfg.PCAP, ifce net.Interface, localizer *i18n.Localizer) (*device.NetworkConfig, error) {
	msgs := localizer.GetMessages()
	// Parse network CIDR
	_, network, err := net.ParseCIDR(pcapCfg.Network)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgs.ParseCIDRError, err)
	}

	// Parse local IP
	localIP := net.ParseIP(pcapCfg.LocalIP)
	if localIP == nil {
		return nil, fmt.Errorf("%s: %s", msgs.ParseIPError, pcapCfg.LocalIP)
	}

	localIP = localIP.To4()
	if !network.Contains(localIP) {
		return nil, fmt.Errorf(msgs.LocalIPNotInNetwork, localIP, network)
	}

	// Parse or use interface MAC
	var localMAC net.HardwareAddr
	if pcapCfg.LocalMAC != "" {
		localMAC, err = net.ParseMAC(pcapCfg.LocalMAC)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", msgs.ParseMACError, err)
		}
	} else {
		localMAC = ifce.HardwareAddr
	}

	// Set MTU
	mtu := pcapCfg.MTU
	if mtu == 0 {
		mtu = uint32(ifce.MTU)
	}

	return &device.NetworkConfig{
		Network:  network,
		LocalIP:  localIP,
		LocalMAC: localMAC,
		MTU:      mtu,
	}, nil
}

func displayNetworkConfig(config *device.NetworkConfig, localizer *i18n.Localizer) {
	// Calculate IP range
	ipRangeStart, ipRangeEnd := calculateIPRange(config.Network, config.LocalIP)
	recommendedMTU := calculateRecommendedMTU(config.MTU)

	// Log network settings in a cleaner format with localization
	for _, line := range localizer.FormatNetworkConfig(ipRangeStart, ipRangeEnd, config.Network.Mask, config.LocalIP, recommendedMTU) {
		slog.Info(line)
	}
}

// calculateIPRange calculates the usable IP range for the given network
func calculateIPRange(network *net.IPNet, gatewayIP net.IP) (start, end net.IP) {
	networkIP := network.IP.To4()
	start = make(net.IP, 4)
	end = make(net.IP, 4)

	// Get the network size
	ones, bits := network.Mask.Size()
	hostBits := uint32(bits - ones)
	numHosts := (uint32(1) << hostBits) - 2 // -2 for network and broadcast

	// Calculate start IP (network + 1)
	binary.BigEndian.PutUint32(start, binary.BigEndian.Uint32(networkIP)+1)

	// Calculate end IP (broadcast - 1)
	broadcastInt := binary.BigEndian.Uint32(networkIP) | ((1 << hostBits) - 1)
	binary.BigEndian.PutUint32(end, broadcastInt-1)

	// Exclude gateway IP from the range
	if bytes.Equal(start, gatewayIP) && numHosts > 1 {
		binary.BigEndian.PutUint32(start, binary.BigEndian.Uint32(start)+1)
	} else if bytes.Equal(end, gatewayIP) && numHosts > 1 {
		binary.BigEndian.PutUint32(end, binary.BigEndian.Uint32(end)-1)
	}

	return start, end
}

// calculateRecommendedMTU returns a recommended MTU value
func calculateRecommendedMTU(mtu uint32) uint32 {
	const ethernetHeaderSize = 14

	// Account for common overhead
	recommendedMTU := mtu - ethernetHeaderSize

	return recommendedMTU
}

func openConfigInEditor() {
	// Get config file path
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", slog.Any("err", err))
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Create config if it doesn't exist
	if !cfg.Exists(cfgFile) {
		// Use default language for this early message
		localizer := i18n.NewLocalizer(i18n.DefaultLanguage)
		msgs := localizer.GetMessages()
		slog.Info(msgs.ConfigNotFound, "file", cfgFile)
		err := os.WriteFile(cfgFile, []byte(configData), 0666)
		if err != nil {
			slog.Error(msgs.ConfigWriteError, slog.Any("file", cfgFile), slog.Any("err", err))
			return
		}
	}

	// Determine the editor command based on OS
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad", cfgFile)
	case "darwin":
		cmd = exec.Command("open", "-t", cfgFile)
	default: // linux and others
		// Try to use EDITOR env var first
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			// Try common editors
			editors := []string{"nano", "vim", "vi"}
			for _, e := range editors {
				if _, err := exec.LookPath(e); err == nil {
					editor = e
					break
				}
			}
		}
		if editor == "" {
			slog.Error("no editor found. Set EDITOR environment variable")
			return
		}
		cmd = exec.Command(editor, cfgFile)
	}

	// Set up the command to use the current terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the editor
	localizer := i18n.NewLocalizer(i18n.DefaultLanguage)
	msgs := localizer.GetMessages()
	slog.Info(msgs.OpeningConfig, "file", cfgFile)
	err = cmd.Run()
	if err != nil {
		slog.Error(msgs.OpenEditorError, slog.Any("err", err))
	}
}
