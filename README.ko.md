<div align="center">

# zwan

**셀프호스팅 Private 오버레이 네트워크 — 내 서버를 띄우고, 어디서든 접속.**

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Release](https://img.shields.io/github/v/release/Zeliper/zwan)](https://github.com/Zeliper/zwan/releases)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux-lightgrey)](https://github.com/Zeliper/zwan/releases)

[← English README](./README.md)

</div>

---

zwan은 누구나 **자기만의** Private 네트워크를 호스팅할 수 있게 한다. 서버를 띄우고(공인 IP 필요)
토큰을 공유하면, 기기들이 어디서든 **암호화된 WireGuard 터널**로 연결된다 — **직접 연결하거나,
NAT 뒤라면 서버의 릴레이를 경유**한다. 서비스는 IP·포트·공유기 설정 대신 **이름**(`nas.home.zwan`)으로 접근한다.

Tailscale/ZeroTier 계열이되, **컨트롤 플레인을 직접 운영**하고 **한 클라이언트가 여러 독립 네트워크에
동시에 가입**할 수 있다. 오픈소스, 중앙 의존 없음.

## 특징

- 🔐 **암호화 오버레이** — WireGuard 데이터플레인(userspace `wireguard-go` + Windows Wintun)
- 🏠 **셀프호스팅** — 자체 컨트롤 서버 + 릴레이. 제3자 개입 없음
- 🌐 **NAT 뒤에서도 동작** — 직접 경로가 없으면 공인 IP 서버의 릴레이로 터널링
- 🧭 **이름 기반 접속** — Split-DNS 리졸버가 `서비스.<서픽스>` 를 올바른 노드로 해석
- 🚪 **L4 서비스 라우터** — 서비스 게시 시 실제 백엔드는 `127.0.0.1`에만 바인딩(LAN/인터넷 미노출)
- 🔀 **다중 네트워크 클라이언트** *(진행 중)* — 한 클라이언트, 여러 독립 네트워크, 주소/DNS 충돌 없이
- 🖥️ **데스크톱 앱** — 시스템 트레이 클라이언트 + **시스템 다크/라이트 테마**. 터널은 SYSTEM 서비스가 담당
- ⬆️ **자동 업데이트** — 클라이언트는 GitHub 릴리스로 자동 업데이트, 서버도 자가 업데이트(`--auto-update`)

## 동작 방식

컨트롤 플레인(서버)은 멤버십·엔드포인트·서비스·DNS 정보만 교환한다. 데이터플레인은 P2P WireGuard이며,
직접 도달이 안 되면 공인 IP를 가진 서버가 패킷을 릴레이한다. Windows에서는 터널을 **SYSTEM 서비스**
(`zwan-service`)가 돌리고, **트레이/GUI**는 사용자 권한으로 named-pipe를 통해 서비스와 통신한다.

## 설치

[Releases](https://github.com/Zeliper/zwan/releases)에서 받는다.

- **Windows 클라이언트** — `zwan-setup.exe`: 트레이 앱 + SYSTEM 엔진 서비스 + Wintun 가상 WAN
  드라이버를 설치하고 로그인 시 시작. 이후 자동 업데이트. *(미서명 빌드라 SmartScreen 경고 시 "추가 정보 → 실행")*
- **서버**(공인 IP 머신에서 호스팅) — 헤드리스 바이너리:
  `zwan-server-linux-amd64/arm64`, `zwan-server-windows-amd64/arm64.exe`

## 빠른 시작

**1. 네트워크 호스팅**(공인 IP 머신):
```bash
./zwan-server-linux-amd64 \
  --token YOUR-SECRET-TOKEN \
  --network home --dns-suffix home.zwan \
  --addr 0.0.0.0:8787 --relay-public 공인IP:3478 --auto-update
```
방화벽에서 TCP `8787`(컨트롤), UDP `3478`(릴레이)을 연다.

**2. 클라이언트 가입:** `zwan-setup.exe` 실행 → zwan 열기 → **Join** → 서버/토큰 입력.
데스크톱에서 직접 호스팅하려면 **Host** 탭 사용(토큰 자동 생성).

**3. 서비스 게시**(백엔드는 localhost 유지):
```bash
zwan-agent --server http://공인IP:8787 --token YOUR-SECRET-TOKEN \
  --device my-nas --name nas --up --relay \
  --publish-name minecraft --publish-port 25565 --publish-backend-port 31001
```
다른 멤버는 `minecraft.home.zwan:25565` 이름으로 접근하고, 실제 백엔드는 `127.0.0.1`에만 열려 있다.

## 소스 빌드

Go 1.23+, Node 18+, (설치파일용) NSIS, (GUI용) Wails CLI 필요.
```bash
go build ./... && go test ./...
cd gui && wails build              # 데스크톱 앱
scripts/build-release.sh 0.1.1     # 전체 릴리스 산출물 -> dist/
```

## 상태 & 로드맵

동작: 컨트롤 플레인, 암호 터널(직접+릴레이), Split-DNS+서비스 레지스트리, L4 라우터, 데스크톱 앱
(트레이+서비스+IPC), Windows 설치파일, 자동 업데이트. 남음: 컨트롤 API TLS/ACME(현재 평문 HTTP),
ACL, 클라이언트-로컬 VIP·UDP 프록시, IPv6 전송, 코드 서명.

설계·계획: [`구현계획.md`](./구현계획.md), [`MyWAN_가상네트워크_아이디어_정리.md`](./MyWAN_가상네트워크_아이디어_정리.md)

## 라이선스

[Apache-2.0](./LICENSE). 의존성은 퍼미시브(MIT/BSD/Apache/ISC)만 사용 — GPL/AGPL/LGPL 금지
([`THIRD_PARTY_NOTICES.md`](./THIRD_PARTY_NOTICES.md)).
