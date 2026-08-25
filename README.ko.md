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
- 🔏 **기본 TLS** — 도메인이 있으면 Let's Encrypt 인증서 자동 발급, 없으면 자체 서명 키 + 지문(pin) 검증
- 🌐 **NAT 뒤에서도 동작** — 직접 경로가 없으면 공인 IP 서버의 릴레이로 터널링
- 🧭 **이름 기반 접속** — Split-DNS 리졸버가 `서비스.<서픽스>` 를 올바른 노드로 해석
- 🚪 **서비스 전용 주소** — 게시한 서비스마다 자기 주소를 받아, 이름만으로 도달하고 두 서비스가 같은 포트를 써도 된다. TCP/UDP 모두. 실제 백엔드는 `127.0.0.1`에만 바인딩
- 👥 **그룹 ACL** — 그룹마다 별도 가입 토큰 + 그룹 간 규칙. 접근 불가한 피어의 키는 애초에 내려주지 않음
- 🔀 **다중 네트워크 클라이언트** — 여러 독립 네트워크에 동시 가입. 네트워크마다 어댑터·키·UDP 포트·이름공간이 분리되고 서로 라우팅되지 않는다
- 🖥️ **데스크톱 앱** — 가입/호스팅 두 역할을 한 트레이 앱에서 + **시스템 다크/라이트 테마**. 실제 동작은 SYSTEM 서비스가 담당
- ⬆️ **자동 업데이트** — 클라이언트는 GitHub 릴리스로 자동 업데이트, 서버도 자가 업데이트(`--auto-update`)

## 동작 방식

컨트롤 채널은 **TLS**다. 도메인이 있으면 ACME로 인증서를 자동 발급받고, 없으면 서버가 자체 서명 키를
유지하면서 그 공개키 지문(pin)을 공개한다. 클라이언트는 CA 체인 대신 이 지문을 검증하므로 IP만 있는
서버도 신원 확인이 된다.

컨트롤 플레인(서버)은 멤버십·엔드포인트·서비스·DNS 정보만 교환한다. 데이터플레인은 P2P WireGuard이며,
직접 도달이 안 되면 공인 IP를 가진 서버가 패킷을 릴레이한다. Windows에서는 두 역할 모두 **SYSTEM 서비스**로 돈다 — 터널은 `zwanEngine`(`zwan-service.exe`),
호스팅은 `zwanServer`(`zwan-server.exe`). **트레이/GUI**는 사용자 권한으로 각각의 named-pipe를 통해
통신하므로, 창을 닫아도 터널과 호스팅은 그대로 유지된다.

## 설치

[Releases](https://github.com/Zeliper/zwan/releases)에서 받는다.

- **Windows** — `zwan-setup.exe`: 트레이 앱을 깔고 역할을 고른다(둘 다 선택 가능).
  - **Client** — `zwanEngine` 서비스 + Wintun 가상 WAN 드라이버. 네트워크 가입용.
  - **Server** — `zwanServer` 서비스. 네트워크 호스팅용. 이것만 고르면 터널 드라이버 없이 서버만 설치된다.

  로그인 시 시작하고 이후 자동 업데이트. *(미서명 빌드라 SmartScreen 경고 시 "추가 정보 → 실행")*
- **리눅스 서버** — 헤드리스 바이너리: `zwan-server-linux-amd64/arm64`
  (`zwan-server-windows-amd64/arm64.exe` 도 있다. 서비스 대신 터미널에서 직접 띄우고 싶을 때)

## 빠른 시작

**1. 네트워크 호스팅**(공인 IP 머신):

*도메인이 있으면* — ACME로 공인 인증서가 자동 발급된다:
```bash
./zwan-server-linux-amd64 \
  --token YOUR-SECRET-TOKEN \
  --network home --dns-suffix home.zwan \
  --addr 0.0.0.0:443 --domain vpn.example.com \
  --relay-public vpn.example.com:3478 --auto-update
```

*도메인이 없으면* — 자체 서명 키를 유지하고, 클라이언트가 검증할 지문을 출력한다:
```bash
./zwan-server-linux-amd64 \
  --token YOUR-SECRET-TOKEN \
  --network home --dns-suffix home.zwan \
  --addr 0.0.0.0:8787 --public-host 공인IP:8787 --relay-public 공인IP:3478
```

어느 쪽이든 나눠줄 주소를 지문까지 붙여 출력한다:
```text
clients join with: --server "https://공인IP:8787#sha256:NMuaxTGRTKlRnx..." --token <token>
```

방화벽에서 컨트롤 포트(TCP `443` 또는 `8787`)와 UDP `3478`(릴레이)을 연다. ACME를 443 이외의
포트에서 쓰면 HTTP-01 챌린지용 TCP `80`도 열어야 한다.

**2. 클라이언트 가입:** `zwan-setup.exe` 실행 → zwan 열기 → **Join** → 출력된 가입 주소를 **Server**
칸에 그대로 붙여넣고(지문은 자동 분리됨) 토큰 입력. 데스크톱에서 직접 호스팅하려면 **Host** 탭 사용 — `zwanServer` 서비스를 설정한다
(네트워크·TLS 모드·토큰, 가입 주소 자동 생성). 창을 닫아도 계속 호스팅되며, 중지는 트레이에서도 된다.

**3. 서비스 게시**(백엔드는 localhost 유지):
```bash
zwan-agent --server "https://공인IP:8787#sha256:NMuaxTGRTKlRnx..." --token YOUR-SECRET-TOKEN \
  --device my-nas --name nas --up --relay \
  --publish-name minecraft --publish-port 25565 --publish-backend-port 31001
```
서버가 서비스마다 **전용 주소**를 주므로, 멤버는 게임이 원래 쓰는 포트 그대로 이름으로 접근한다 —
`minecraft.home.zwan`, 포트 25565. 따로 찾아볼 것이 없다. 같은 머신에 같은 포트로 하나 더 띄워도
주소가 다르니 충돌하지 않는다.

```text
minecraft.home.zwan   100.64.128.1:25565   ->  127.0.0.1:31001
survival.home.zwan    100.64.128.2:25565   ->  127.0.0.1:31002
voice.home.zwan       100.64.128.3:64738   ->  127.0.0.1:31003   (udp)
```

실제 백엔드는 내내 `127.0.0.1`에만 열려 있다. 기기는 오버레이 대역의 아래쪽 절반, 서비스는 위쪽 절반에서
번호를 받으므로 둘이 겹칠 수 없다.

## 여러 네트워크 동시 가입

한 기기가 여러 네트워크에 동시에 속할 수 있다. 원래 하나뿐인 자원은 의도적으로 분리한다 —
네트워크마다 **Wintun 어댑터·노드 키·UDP 포트**가 따로라, 한쪽 트래픽이 다른 쪽으로 샐 여지가 없다.
DNS 리졸버는 하나다(`127.0.0.1:53` 은 한 번만 점유 가능). 대신 네트워크마다 존으로 나뉜다.

이름은 서버가 알려주는 서픽스가 아니라 **가입할 때 정하는 로컬 이름**으로 구분한다.
두 서버가 똑같이 `home.zwan` 을 쓸 수도 있기 때문이다.

```text
alice 서버 가입, 로컬 이름 "alice"   ->  nas.alice
bob   서버 가입, 로컬 이름 "bob"     ->  nas.bob
```

주소는 **네트워크별로 변환**된다. 그래서 두 네트워크가 같은 오버레이 대역을 써도(기본값이 모두
같으니 흔한 일이다) 여기서는 충돌하지 않는다. 네트워크마다 로컬 풀(`100.112.0.0/12` 기본)에서
한 조각을 받고, 기기는 그 안의 로컬 주소를 갖고 터널은 실제 주소를 나른다.

```text
alice   이 기기 100.112.0.1   네트워크 안 100.64.0.1
bob     이 기기 100.113.0.1   네트워크 안 100.64.0.1   (같지만 문제 없음)
```

호스트 라우팅 테이블에는 로컬 주소만 올라가므로, 서버가 그 풀을 자기 오버레이 대역으로 써도 무방하다.
변환을 끌 수도 있는데, 그러면 겹치는 네트워크를 해결하는 대신 경고만 표시한다.

## 접근 제어

기본은 전원이 서로 접근 가능하다(가정용에는 이게 맞다). 나누려면 그룹마다 가입 토큰을 따로 주고
그룹 간 규칙을 쓴다.

```bash
./zwan-server-linux-amd64 --token FAMILY-TOKEN \
  --join-token dev=DEV-TOKEN --join-token guest=GUEST-TOKEN --join-token nas=NAS-TOKEN \
  --acl "dev->nas" --acl "guest->nas"
```

**어떤 토큰으로 가입했는지가 그룹을 결정한다** — 클라이언트가 보내는 값이 아니다. 규칙이 하나라도
있으면 나머지는 전부 차단이므로, 위 예에서 `dev`·`guest` 는 각각 `nas` 에만 닿고 서로는 못 본다.
큰 정책은 `--acl-file` 로 JSON 파일을 쓰고, 데스크톱 앱 **Host** 탭에서도 같은 그룹·규칙을 편집한다.

강제 방식은 필터링이 아니라 **누락**이다. 접근 불가한 피어는 목록에 아예 안 나오고, 피어의 공개키가
없으면 터널 자체가 성립하지 않는다. 서비스는 호스트 노드보다 더 좁힐 수 있다.

```bash
zwan-agent ... --publish-name files --publish-port 445 \
  --publish-backend-port 31445 --publish-allow dev
```

이건 두 번 검사된다 — 컨트롤 서버가 다른 그룹에게 숨기고, **호스팅 노드가 그 그룹의 접속을 거부**한다.
주소와 포트를 알아도 못 붙는다.

## 소스 빌드

Go 1.23+, Node 18+, (설치파일용) NSIS, (GUI용) Wails CLI 필요.
```bash
go build ./... && go test ./...
cd gui && wails build              # 데스크톱 앱
scripts/build-release.sh 0.1.1     # 전체 릴리스 산출물 -> dist/
```

## 상태 & 로드맵

동작: 컨트롤 플레인 TLS(ACME 또는 지문 핀닝 자체서명), 그룹 ACL, 다중 네트워크, 서비스 전용 주소(TCP/UDP), 암호 터널(직접+릴레이),
Split-DNS(NRPT 로 시스템 이름 해석에 연결)+서비스 레지스트리, L4 라우터, 데스크톱 앱(트레이+서비스+IPC), Windows 설치파일, 자동 업데이트.
남음: NAT 트래버설, IPv6 전송, 코드 서명.

## 보안

컨트롤 API는 기본이 TLS다(도메인 시 ACME, 아니면 자체서명 + 지문 핀닝). 지문은 신뢰할 수 있는
경로로 전달한다(출력되는 가입 주소에 이미 포함돼 있다). `--tls=off` 는 로컬 테스트/리버스 프록시용이며
토큰이 평문으로 나간다.

가입 토큰은 **가입 권한만** 준다. 등록하면 기기별 노드 토큰이 발급되고, 멤버·서비스 목록 조회에는
이 토큰이 필요하다 — API에 닿는 것만으로는 네트워크를 열람할 수 없고, 멤버는 **자기 노드에만**
서비스를 게시할 수 있다.

설계·계획: [`구현계획.md`](./구현계획.md), [`MyWAN_가상네트워크_아이디어_정리.md`](./MyWAN_가상네트워크_아이디어_정리.md)

## 라이선스

[Apache-2.0](./LICENSE). 의존성은 퍼미시브(MIT/BSD/Apache/ISC)만 사용 — GPL/AGPL/LGPL 금지
([`THIRD_PARTY_NOTICES.md`](./THIRD_PARTY_NOTICES.md)).
