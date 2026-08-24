# MyWAN (repo: `zwan`)

오픈소스 셀프호스팅 오버레이 네트워크. 누구나 자기 서버를 호스팅해(공인 IP 필요) 자기만의 Private
네트워크를 만들고, 클라이언트는 **여러 네트워크에 동시에 가입**해 이름만으로 서비스에 접속한다.

Tailscale/ZeroTier 계열이되, **중앙 의존 없이 각자 셀프호스팅**하고 **한 클라이언트가 여러 독립
네트워크에 동시 가입**하는 것이 핵심 차별점이다.

> **이름 규칙:** 기술 식별자(모듈 `github.com/Zeliper/zwan`, 바이너리 `zwan-server`/`zwan-agent`)는
> 안정적으로 고정한다. **브랜드명(현재 "MyWAN")은 `shared.ProductName` 한 곳에서 언제든 변경**할 수 있다
> (또는 빌드 시 `-ldflags "-X github.com/Zeliper/zwan/shared.ProductName=NewName"`).

## 구성
- **zwan-server** — 셀프호스팅 Control Plane (인증/IPAM/DNS/서비스 레지스트리/ACL/STUN/Relay).
  Windows 데스크톱 앱(GUI+트레이) 또는 Linux VPS 헤드리스로 구동.
- **zwan-agent** — Windows 클라이언트 (가상 NIC, 암호화 터널, Split DNS, L4 서비스 라우팅, 다중 네트워크).
- **frontend** — Wails v2 기반 GUI (React + shadcn/ui, 시스템 다크/라이트 테마).

## 설치 (Releases)
[Releases](https://github.com/Zeliper/zwan/releases)에서 받는다.
- **Windows 클라이언트**: `zwan-setup.exe` — 트레이 앱 + SYSTEM 엔진 서비스 + Wintun 가상 WAN 드라이버를 설치하고 로그인 시 시작. 이후 자동 업데이트.
- **서버(자가호스팅)**: `zwan-server-windows-amd64.exe` / `zwan-server-linux-amd64` — 공인 IP 머신(VPS)용 헤드리스 컨트롤 서버 + 릴레이.

아키텍처: 엔진 = SYSTEM 서비스(`cmd/zwan-service`), 트레이/GUI = 사용자 권한, 둘 사이는 named-pipe IPC.

## 문서
- 설계: [`MyWAN_가상네트워크_아이디어_정리.md`](./MyWAN_가상네트워크_아이디어_정리.md)
- 구현 계획: [`구현계획.md`](./구현계획.md)

## 스택
Go · wireguard-go · Wintun · Wails v2(React/TS/Vite/Tailwind/shadcn) · SQLite(modernc.org/sqlite)

## 개발 준비물
```
Go 1.23+     winget install --id GoLang.Go -e     (설치됨: 1.26.7)
Node 18+     (설치됨: 24.x)
Wails CLI    go install github.com/wailsapp/wails/v2/cmd/wails@latest   (GUI 단계에서)
gcc(MSYS2)   cgo용 (설치됨)
```

## 빌드
```
# 코어 (서버 + 클라이언트 CLI)
go build ./...
go test ./...
go run ./cmd/zwan-server --token demo-token-123   # control :8787, relay :3478
go run ./cmd/zwan-agent  --token demo-token-123 --device pc-1 --name pc1
#   --up : 어댑터 + WireGuard 터널(관리자)   --relay : 서버 릴레이 경유
#   --publish-name minecraft --publish-port 25565 --publish-backend-port 31001

# 데스크톱 GUI (Wails v2, gui/ 는 별도 모듈)
cd gui && wails build     # -> gui/build/bin/gui.exe  (또는 wails dev 로 핫리로드)
```

## 라이선스
Apache License 2.0 — [`LICENSE`](./LICENSE) 참조. 제3자 라이브러리 고지는
[`THIRD_PARTY_NOTICES.md`](./THIRD_PARTY_NOTICES.md).
