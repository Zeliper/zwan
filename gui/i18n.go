package main

import (
	"strings"
	"sync"
)

// The tray is drawn outside the window and may be the only part of this program
// a user sees for days at a time, so it is translated too.
//
// It keeps its own small table rather than sharing the window's: the two
// surfaces have almost no strings in common, and the tray has to be readable
// before the window has ever been opened — the language then comes from the
// operating system, and the window replaces it with the user's own choice as
// soon as it loads.
type lang string

const (
	langEN lang = "en"
	langKO lang = "ko"
)

type trayText struct {
	show          string
	showTip       string
	stopHosting   string
	stopHostTip   string
	quit          string
	quitTip       string
	tooltip       string
	idle          string
	joinedPrefix  string
	hostingPrefix string
	disconnected  string
	connected     string
	leave         string
	join          string
}

var trayStrings = map[lang]trayText{
	langEN: {
		show:          "Show",
		showTip:       "Show the zwan window",
		stopHosting:   "Stop hosting",
		stopHostTip:   "Stop the network this machine hosts",
		quit:          "Quit",
		quitTip:       "Quit zwan",
		tooltip:       "zwan overlay network",
		idle:          "zwan — idle",
		joinedPrefix:  "joined ",
		hostingPrefix: "hosting ",
		disconnected:  "disconnected",
		connected:     "connected",
		leave:         "Leave ",
		join:          "Join ",
	},
	langKO: {
		show:          "창 열기",
		showTip:       "zwan 창을 엽니다",
		stopHosting:   "호스팅 중지",
		stopHostTip:   "이 기기가 호스팅 중인 네트워크를 멈춥니다",
		quit:          "종료",
		quitTip:       "zwan 종료",
		tooltip:       "zwan 오버레이 네트워크",
		idle:          "zwan — 대기 중",
		joinedPrefix:  "가입: ",
		hostingPrefix: "호스팅: ",
		disconnected:  "연결 안 됨",
		connected:     "연결됨",
		leave:         "나가기: ",
		join:          "가입: ",
	},
}

var (
	langMu   sync.RWMutex
	current  = langEN
	onChange []func()
)

// text returns the tray strings for the language in force.
func text() trayText {
	langMu.RLock()
	defer langMu.RUnlock()
	return trayStrings[current]
}

// SetLanguage switches the interface language. The window calls this whenever
// its own language changes — including once at startup — so the tray follows the
// user's choice rather than only the operating system's.
func (a *App) SetLanguage(code string) {
	next := langEN
	if strings.HasPrefix(strings.ToLower(code), "ko") {
		next = langKO
	}

	langMu.Lock()
	if next == current {
		langMu.Unlock()
		return
	}
	current = next
	hooks := append([]func(){}, onChange...)
	langMu.Unlock()

	for _, fn := range hooks {
		fn()
	}
}

// onLanguageChange registers a relabelling hook. systray can rename an item but
// not rebuild the menu, so each surface re-applies its own titles.
func onLanguageChange(fn func()) {
	langMu.Lock()
	onChange = append(onChange, fn)
	langMu.Unlock()
}

// initLanguage seeds the language from the operating system, for the window that
// has not opened yet.
func initLanguage() {
	langMu.Lock()
	if strings.HasPrefix(strings.ToLower(systemLanguage()), "ko") {
		current = langKO
	}
	langMu.Unlock()
}
