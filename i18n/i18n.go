// Package i18n provides internationalization support for go-pcap2socks.
package i18n

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// Pre-defined errors for i18n operations
var (
	ErrUnsupportedLanguage = errors.New("unsupported language")
	ErrStackNotInitialized = errors.New("stack not initialized")
)

// Language represents a supported language code.
type Language string

const (
	// English language code.
	English Language = "en"
	// Russian language code.
	Russian Language = "ru"
	// Chinese language code.
	Chinese Language = "zh"
	// Default language (English).
	DefaultLanguage = English
)

// Messages contains all translatable messages.
type Messages struct {
	// Startup messages
	ConfigLoaded          string
	ConfigNotFound        string
	ConfigCreating        string
	ConfigWriteError      string
	LoadConfigError       string
	ExecutingCommands     string
	ExecuteCommandError   string
	RunError              string
	UsingInterface        string
	ConfigureDevice       string
	IPAddress             string
	SubnetMask            string
	Gateway               string
	MTU                   string
	ConfigSeparator       string
	NetworkConfigError    string
	LocalIPNotInNetwork   string
	ParseCIDRError        string
	ParseIPError          string
	ParseMACError         string
	InterfaceNotFound     string
	DiscoverInterfaceError string
	CreateStackError      string
	NewSocks5Error        string
	InvalidOutbound       string
	OpeningConfig         string
	NoEditorFound         string
	OpenEditorError       string
	HelloWorld            string
	ListenError           string
}

// English messages.
var enMessages = Messages{
	ConfigLoaded:          "Config loaded",
	ConfigNotFound:        "Config file not found, creating a new one",
	ConfigCreating:        "Creating config file",
	ConfigWriteError:      "Failed to write config file",
	LoadConfigError:       "Failed to load config",
	ExecutingCommands:     "Executing commands on start",
	ExecuteCommandError:   "Failed to execute command",
	RunError:              "Run error",
	UsingInterface:        "Using ethernet interface",
	ConfigureDevice:       "Configure your device with these network settings:",
	IPAddress:             "IP Address",
	SubnetMask:            "Subnet Mask",
	Gateway:               "Gateway",
	MTU:                   "MTU",
	ConfigSeparator:       "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
	NetworkConfigError:    "Failed to parse network config",
	LocalIPNotInNetwork:   "Local IP (%s) not in network (%s)",
	ParseCIDRError:        "Failed to parse CIDR",
	ParseIPError:          "Failed to parse IP",
	ParseMACError:         "Failed to parse MAC",
	InterfaceNotFound:     "Interface with IP %s not found",
	DiscoverInterfaceError: "Failed to discover default gateway",
	CreateStackError:      "Failed to create stack",
	NewSocks5Error:        "Failed to create SOCKS5 proxy",
	InvalidOutbound:       "Invalid outbound configuration",
	OpeningConfig:         "Opening config in editor",
	NoEditorFound:         "No text editor found. Please set EDITOR environment variable",
	OpenEditorError:       "Failed to open editor",
	HelloWorld:            "Hello world!",
	ListenError:           "Failed to start HTTP server",
}

// Russian messages.
var ruMessages = Messages{
	ConfigLoaded:          "Конфигурация загружена",
	ConfigNotFound:        "Файл конфигурации не найден, создаётся новый",
	ConfigCreating:        "Создание файла конфигурации",
	ConfigWriteError:      "Не удалось записать файл конфигурации",
	LoadConfigError:       "Не удалось загрузить конфигурацию",
	ExecutingCommands:     "Выполнение команд при запуске",
	ExecuteCommandError:   "Не удалось выполнить команду",
	RunError:              "Ошибка запуска",
	UsingInterface:        "Используется сетевой интерфейс",
	ConfigureDevice:       "Настройте ваше устройство со следующими параметрами:",
	IPAddress:             "IP адрес",
	SubnetMask:            "Маска подсети",
	Gateway:               "Шлюз",
	MTU:                   "MTU",
	ConfigSeparator:       "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
	NetworkConfigError:    "Не удалось разобрать конфигурацию сети",
	LocalIPNotInNetwork:   "Локальный IP (%s) не в сети (%s)",
	ParseCIDRError:        "Не удалось разобрать CIDR",
	ParseIPError:          "Не удалось разобрать IP",
	ParseMACError:         "Не удалось разобрать MAC",
	InterfaceNotFound:     "Интерфейс с IP %s не найден",
	DiscoverInterfaceError: "Не удалось обнаружить шлюз по умолчанию",
	CreateStackError:      "Не удалось создать стек",
	NewSocks5Error:        "Не удалось создать SOCKS5 прокси",
	InvalidOutbound:       "Неверная конфигурация исходящего соединения",
	OpeningConfig:         "Открытие конфигурации в редакторе",
	NoEditorFound:         "Текстовый редактор не найден. Установите переменную окружения EDITOR",
	OpenEditorError:       "Не удалось открыть редактор",
	HelloWorld:            "Привет мир!",
	ListenError:           "Не удалось запустить HTTP сервер",
}

// Chinese messages.
var zhMessages = Messages{
	ConfigLoaded:          "配置已加载",
	ConfigNotFound:        "配置文件不存在，正在创建新文件",
	ConfigCreating:        "创建配置文件",
	ConfigWriteError:      "写入配置文件失败",
	LoadConfigError:       "加载配置失败",
	ExecutingCommands:     "正在执行启动命令",
	ExecuteCommandError:   "执行命令失败",
	RunError:              "运行错误",
	UsingInterface:        "正在使用以太网接口",
	ConfigureDevice:       "请使用以下网络设置配置您的设备：",
	IPAddress:             "IP 地址",
	SubnetMask:            "子网掩码",
	Gateway:               "网关",
	MTU:                   "MTU",
	ConfigSeparator:       "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
	NetworkConfigError:    "解析网络配置失败",
	LocalIPNotInNetwork:   "本地 IP (%s) 不在网络 (%s) 中",
	ParseCIDRError:        "解析 CIDR 失败",
	ParseIPError:          "解析 IP 失败",
	ParseMACError:         "解析 MAC 失败",
	InterfaceNotFound:     "未找到 IP 为 %s 的接口",
	DiscoverInterfaceError: "发现默认网关失败",
	CreateStackError:      "创建协议栈失败",
	NewSocks5Error:        "创建 SOCKS5 代理失败",
	InvalidOutbound:       "无效的出站配置",
	OpeningConfig:         "正在编辑器中打开配置文件",
	NoEditorFound:         "未找到文本编辑器。请设置 EDITOR 环境变量",
	OpenEditorError:       "打开编辑器失败",
	HelloWorld:            "你好世界！",
	ListenError:           "启动 HTTP 服务器失败",
}

// Localizer provides localized messages.
type Localizer struct {
	lang     Language
	messages *Messages
}

// NewLocalizer creates a new Localizer for the specified language.
// If lang is empty, it tries to get language from PCAP2SOCKS_LANG environment variable.
// Falls back to English if no valid language is specified.
func NewLocalizer(lang Language) *Localizer {
	if lang == "" {
		// Try to get language from environment variable
		envLang := os.Getenv("PCAP2SOCKS_LANG")
		if envLang != "" {
			lang = Language(strings.ToLower(envLang[:2]))
		}
		if lang == "" {
			lang = DefaultLanguage
		}
	}

	switch lang {
	case Russian:
		return &Localizer{lang: Russian, messages: &ruMessages}
	case Chinese:
		return &Localizer{lang: Chinese, messages: &zhMessages}
	case English:
		fallthrough
	default:
		return &Localizer{lang: English, messages: &enMessages}
	}
}

// GetMessages returns the messages for the current language.
func (l *Localizer) GetMessages() *Messages {
	return l.messages
}

// Language returns the current language code.
func (l *Localizer) Language() Language {
	return l.lang
}

// FormatNetworkConfig formats network configuration message with localization.
func (l *Localizer) FormatNetworkConfig(ipRangeStart, ipRangeEnd net.IP, mask net.IPMask, gateway net.IP, mtu uint32) []string {
	msgs := l.messages
	return []string{
		msgs.ConfigureDevice,
		msgs.ConfigSeparator,
		fmt.Sprintf("  %s:     %s - %s", msgs.IPAddress, ipRangeStart.String(), ipRangeEnd.String()),
		fmt.Sprintf("  %s:    %s", msgs.SubnetMask, net.IP(mask).String()),
		fmt.Sprintf("  %s:        %s", msgs.Gateway, gateway.String()),
		fmt.Sprintf("  %s:            %d (or lower)", msgs.MTU, mtu),
		msgs.ConfigSeparator,
	}
}

// GetStackStatusMessage returns a localized message for stack status.
// Returns ErrStackNotInitialized if stack is nil.
func (l *Localizer) GetStackStatusMessage(s *stack.Stack) (string, error) {
	if s == nil {
		return "", ErrStackNotInitialized
	}
	switch l.lang {
	case Russian:
		return "Стек инициализирован", nil
	case Chinese:
		return "协议栈已初始化", nil
	default:
		return "Stack initialized", nil
	}
}
