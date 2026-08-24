# MyWAN 가상 네트워크 아이디어 정리

## 1. 프로젝트 개요

이 아이디어는 Windows에 가상 네트워크 인터페이스를 설치하고, 중앙 Control Server가 가입자/장치/서비스 정보를 관리하며, 실제 데이터 통신은 가능한 경우 가입자 간 P2P로 직접 처리하는 Overlay Network 구조이다.

개념적으로는 다음 기술들의 일부 특징을 결합한 형태에 가깝다.

- Tailscale / ZeroTier 계열 Overlay Network
- WireGuard 기반 암호화 터널
- 중앙 Control Plane
- Split DNS
- Service Discovery
- Virtual Service IP
- L4 TCP/UDP Port Mapping
- 필요 시 L7 Reverse Proxy
- NAT Traversal
- STUN
- Relay Fallback

핵심 목표는 다음과 같다.

> 사용자는 실제 공인 IP, 공유기 포트포워딩, 실제 서비스 포트, 서버의 물리 위치를 몰라도 도메인 이름만으로 서비스에 접속할 수 있다.

예:

```text
minecraft.mywan
nas.mywan
office-pc.mywan
postgres.mywan
```

---

## 2. 기본 구조

전체 시스템은 크게 Control Plane과 Data Plane으로 나뉜다.

```text
                     Control Server
                ┌─────────────────────┐
                │ Authentication      │
                │ Device Registry     │
                │ IPAM                │
                │ Internal DNS        │
                │ Peer Discovery      │
                │ Endpoint Directory  │
                │ ACL                 │
                │ Service Registry    │
                │ STUN                │
                │ Relay Fallback      │
                └──────────┬──────────┘
                           │
                    제어 정보만 전달
                           │
               ┌───────────┴───────────┐
               │                       │
             PC-A                    PC-B
          Virtual NIC             Virtual NIC
               │                       │
               └════ Encrypted P2P ════┘
```

Control Server는 가능하면 사용자 트래픽을 직접 중계하지 않는다.

대부분의 데이터는:

```text
PC-A <===================> PC-B
         Direct P2P
```

로 전달한다.

직접 연결이 불가능한 경우에만:

```text
PC-A -> Relay Server -> PC-B
```

형태로 처리한다.

---

## 3. Windows 가상 NIC

Windows에는 MyWAN용 가상 NIC를 생성한다.

초기 구현에서는 커널 드라이버를 직접 개발하기보다 Wintun 같은 Layer 3 가상 인터페이스를 활용하는 것이 현실적이다.

예:

```text
Windows PC

Physical NIC
  192.168.0.20

MyWAN Virtual NIC
  100.64.0.20
```

사용자는 기존 LAN/Wi-Fi와 MyWAN 네트워크를 동시에 가진다.

```text
Physical Network
192.168.x.x
172.16.x.x
10.x.x.x
공인 IPv4/IPv6

+

MyWAN Overlay
100.64.x.x
```

---

## 4. IP 할당 방식

전통적인 DHCP를 실제로 구현할 필요는 없다.

사용자 경험은 DHCP처럼 보이지만 내부적으로는 Control Server의 IPAM이 장치에 가상 IP를 할당하고, MyWAN Client가 Windows API를 통해 가상 NIC에 직접 설정하는 방식이 더 적합하다.

예:

```text
Client 설치
    ↓
로그인
    ↓
Device 등록
    ↓
Control Server
    ↓
IPAM
    ↓
100.64.0.20 할당
    ↓
MyWAN Client가 Virtual NIC 구성
```

따라서 사용자는:

```text
설치 → 로그인 → 완료
```

정도로 끝낼 수 있다.

장치별 고정 IP도 가능하다.

```text
Desktop    -> 100.64.0.10
Laptop     -> 100.64.0.11
NAS        -> 100.64.0.12
GameServer -> 100.64.0.13
```

Device UUID를 기준으로 동일 주소를 재할당하는 방식도 가능하다.

---

## 5. 일반 인터넷에 영향을 주지 않는 구조

MyWAN을 Full Tunnel VPN처럼 사용하지 않고 특정 가상망 대역만 MyWAN Virtual NIC로 라우팅한다.

예:

```text
0.0.0.0/0
  -> 기존 공유기 / Wi-Fi
  -> 일반 인터넷

100.64.0.0/10
  -> MyWAN Virtual NIC

Service VIP Pool
  -> MyWAN Virtual NIC
```

따라서:

```text
google.com
youtube.com
github.com
```

등은 기존 인터넷을 그대로 사용한다.

반면:

```text
minecraft.mywan
server01.mywan
nas.mywan
```

등은 MyWAN을 통해 접속한다.

즉:

```text
Chrome -> google.com -> 기존 Internet

Game -> minecraft.mywan -> MyWAN Overlay
```

형태이다.

---

## 6. Split DNS

Windows 전체 DNS를 MyWAN DNS로 강제로 변경하는 것보다 특정 Namespace만 MyWAN DNS로 전달하는 Split DNS 구조가 적합하다.

예:

```text
*.mywan
*.game.mywan
*.internal.example.com

        ↓

MyWAN DNS
```

그 외:

```text
google.com
naver.com
github.com
```

은 기존 DNS를 그대로 사용한다.

예:

```text
minecraft.mywan
    ↓
MyWAN DNS
    ↓
100.66.0.1

google.com
    ↓
기존 Windows DNS
```

실제 인터넷 DNS에 존재하지 않는 이름도 사용할 수 있다.

다만 임의의 사설 TLD를 사용하는 것보다는 실제 소유한 도메인의 하위 도메인을 내부 DNS 전용으로 사용하는 것도 좋은 방법이다.

예:

```text
minecraft.net.example.com
db.internal.example.com
nas.private.example.com
```

공용 DNS에서는 존재하지 않더라도 MyWAN 가입자에게만 응답할 수 있다.

---

## 7. NAT Traversal

양쪽 PC가 공유기나 CGNAT 뒤에 있더라도 가능한 경우 직접 P2P 연결을 시도한다.

기본 개념:

```text
PC-A
192.168.0.10
   ↓
NAT
   ↓
Public Endpoint

         Internet

Public Endpoint
   ↑
NAT
   ↑
PC-B
192.168.10.20
```

Control Server가 양쪽 Endpoint를 알려주고 동시에 UDP를 보내 NAT Mapping을 생성한다.

```text
PC-A ───── UDP ─────► NAT-B
PC-B ───── UDP ─────► NAT-A
```

이후:

```text
PC-A <========= P2P =========> PC-B
```

로 통신한다.

사용 기술 후보:

- STUN
- UDP Hole Punching
- UPnP
- NAT-PMP
- PCP
- IPv6 Direct
- LAN Direct

---

## 8. P2P 실패 시 Relay

100% P2P 연결을 보장할 수는 없다.

다음 환경에서는 직접 연결에 실패할 수 있다.

- Symmetric NAT
- 강한 기업 방화벽
- UDP 차단
- 일부 통신사 CGNAT
- 다중 NAT
- 특수 네트워크 정책

따라서 Path Selection은 예를 들면 다음 순서로 구성한다.

```text
1. Same LAN
2. Native IPv6 Direct
3. IPv4 Direct
4. UDP NAT Traversal
5. Peer Relay
6. Server Relay
```

Relay는 마지막 수단으로 사용한다.

이 방식의 장점은 중앙 서버의 트래픽 비용을 최소화할 수 있다는 점이다.

---

## 9. TCP 서비스도 UDP 기반 Tunnel로 운반 가능

게임이나 SSH 같은 애플리케이션은 일반 TCP를 사용해도 된다.

예:

```text
SSH
   ↓
TCP
   ↓
MyWAN Virtual NIC
   ↓
Encrypted UDP Tunnel
   ↓
Internet
   ↓
Remote MyWAN
   ↓
TCP
   ↓
SSH Server
```

즉 애플리케이션에는 정상적인 TCP 연결로 보이지만 실제 인터넷 구간에서는 UDP 기반 Overlay Tunnel로 전달할 수 있다.

따라서 다음 서비스를 모두 지원할 수 있다.

- TCP
- UDP
- RDP
- SSH
- SMB
- PostgreSQL
- HTTP/HTTPS
- Minecraft
- Factorio
- Valheim
- 각종 Dedicated Server

---

## 10. Node IP와 Service VIP 분리

이 프로젝트에서 중요한 설계 중 하나이다.

장치 자체의 주소와 서비스의 주소를 분리한다.

예:

```text
Node IP
100.64.0.10
= Main PC 자체

Service VIP
100.66.0.1
= Minecraft

Service VIP
100.66.0.2
= Factorio

Service VIP
100.66.0.3
= PostgreSQL
```

DNS:

```text
main-pc.mywan
  -> 100.64.0.10

minecraft.mywan
  -> 100.66.0.1

factorio.mywan
  -> 100.66.0.2

postgres.mywan
  -> 100.66.0.3
```

이렇게 하면 서비스의 위치와 컴퓨터의 주소를 분리할 수 있다.

---

## 11. 하나의 PC에서 여러 서비스를 같은 기본 포트로 운영

일반 인터넷에서는 공인 IP 하나에 같은 TCP/UDP 포트를 여러 서비스가 동시에 사용할 수 없다.

예:

```text
1.2.3.4:25565
```

는 하나의 목적지밖에 가질 수 없다.

DNS를:

```text
mc-a.example.com -> 1.2.3.4
mc-b.example.com -> 1.2.3.4
```

로 나눠도 일반적인 게임 프로토콜은 두 연결을 구분할 수 없다.

하지만 MyWAN에서는 서비스마다 별도의 VIP를 줄 수 있다.

```text
mc-a.mywan -> 100.66.0.1:25565
mc-b.mywan -> 100.66.0.2:25565
mc-c.mywan -> 100.66.0.3:25565
```

실제 Main PC에서는:

```text
Minecraft A -> 127.0.0.1:31001
Minecraft B -> 127.0.0.1:31002
Minecraft C -> 127.0.0.1:31003
```

MyWAN Agent가 다음처럼 변환한다.

```text
100.66.0.1:25565 -> 127.0.0.1:31001
100.66.0.2:25565 -> 127.0.0.1:31002
100.66.0.3:25565 -> 127.0.0.1:31003
```

따라서 사용자는:

```text
mc-a.mywan
mc-b.mywan
mc-c.mywan
```

만 입력하면 된다.

---

## 12. Service VIP를 실제 Windows NIC에 모두 등록할 필요는 없음

서비스가 수천 개일 경우 Windows NIC에 가상 IP를 하나씩 Alias로 등록하는 방법은 비효율적일 수 있다.

대신 Service CIDR 전체를 MyWAN Adapter로 Routing한다.

예:

```text
100.66.0.0/16
    ↓
MyWAN Virtual NIC
```

그러면 다음 주소는 모두 MyWAN Agent가 받는다.

```text
100.66.0.1
100.66.0.2
100.66.20.50
100.66.255.254
```

MyWAN Agent 내부에서:

```text
Destination VIP
    ↓
Service Table
    ↓
Service ID
    ↓
Target Node
    ↓
Target Backend Port
```

로 처리한다.

즉 Service VIP는 실제 NIC 주소라기보다 논리적인 Service ID 역할을 하게 된다.

---

## 13. 서비스 이동 및 Failover

Service VIP는 실제 실행 위치와 분리되어 있으므로 서버를 이동해도 사용자 설정을 변경할 필요가 없다.

예:

```text
minecraft.mywan
    ↓
100.66.0.1
```

기존:

```text
100.66.0.1
    ↓
MAIN-PC:31001
```

서버 이전 후:

```text
100.66.0.1
    ↓
SERVER-02:42000
```

DNS는 그대로 유지된다.

사용자는 계속:

```text
minecraft.mywan
```

으로 접속한다.

이 구조는 DB Failover 등에도 사용할 수 있다.

```text
postgres.mywan
    ↓
100.66.0.20:5432
    ↓
현재 Primary DB
```

Primary 장애 시 Control Plane에서 Backend만 변경한다.

---

## 14. L4와 L7 역할

게임 서버에는 일반적으로 L7 Proxy보다 L4 Virtual Service가 적합하다.

HTTP/HTTPS는 다음 정보를 이용할 수 있다.

- HTTP Host Header
- TLS SNI

따라서 같은 IP와 Port 443에서도:

```text
git.mywan
wiki.mywan
grafana.mywan
```

을 구분할 수 있다.

하지만 대부분의 게임 프로토콜은 Host 정보를 제공하지 않는다.

따라서 게임에는:

```text
Virtual IP + Protocol + Port
```

를 Routing Key로 사용하는 것이 더 범용적이다.

예:

```text
(VIP, Protocol, Port)

100.66.0.1 / TCP / 25565
100.66.0.2 / UDP / 34197
100.66.0.3 / TCP / 5432
```

---

## 15. 서비스 테이블 예시

```text
SERVICE         VIP           PROTO   VPORT   NODE       BACKEND

Minecraft-A     100.66.0.1    TCP     25565   MAIN-PC    127.0.0.1:31001
Minecraft-B     100.66.0.2    TCP     25565   MAIN-PC    127.0.0.1:31002
Factorio        100.66.0.3    UDP     34197   MAIN-PC    127.0.0.1:31003
Valheim         100.66.0.4    UDP     2456    SERVER-B   192.168.1.20:2456
PostgreSQL      100.66.0.5    TCP     5432    DB-A       127.0.0.1:15432
```

DNS:

```text
minecraft-a.mywan -> 100.66.0.1
minecraft-b.mywan -> 100.66.0.2
factorio.mywan    -> 100.66.0.3
valheim.mywan     -> 100.66.0.4
postgres.mywan    -> 100.66.0.5
```

---

## 16. 실제 Backend Port를 숨길 수 있음

사용자는 서비스의 실제 Port를 알 필요가 없다.

예:

```text
minecraft.mywan
    ↓
100.66.0.1:25565
    ↓
MyWAN Agent
    ↓
127.0.0.1:38124
```

실제 게임 서버가 랜덤하거나 다른 Port를 사용해도 사용자는 게임의 기본 Port만 사용하면 된다.

게임이 기본 Port를 자동 사용한다면 사용자는 도메인만 입력하면 된다.

---

## 17. DNS SRV

일부 프로토콜은 DNS SRV Record를 통해 Port를 전달할 수 있다.

예:

```text
_minecraft._tcp.game.mywan

SRV
priority 0
weight 5
port 38124
host server01.mywan
```

지원하는 클라이언트라면 사용자가:

```text
game.mywan
```

만 입력해도 실제 Port를 찾을 수 있다.

다만 모든 게임이 SRV를 지원하는 것은 아니므로 범용 구조의 핵심으로 의존하기는 어렵다.

Service VIP 방식이 더 범용적이다.

---

## 18. 서비스 접근 제어

Control Plane이 사용자와 서비스를 모두 알고 있으므로 ACL을 적용할 수 있다.

예:

```text
minecraft.company.mywan

Allowed:
  group://developers
  group://friends
```

접속 과정:

```text
User
  ↓
DNS
  ↓
Service VIP
  ↓
MyWAN Agent
  ↓
Authentication
  ↓
ACL Check
  ↓
Backend Discovery
  ↓
P2P 연결
```

권한이 없는 사용자는 실제 서버 IP나 Port를 알더라도 연결을 허용하지 않을 수 있다.

---

## 19. Backend를 localhost에만 Listen 가능

게임 서버나 DB를:

```text
0.0.0.0:31001
```

에 열지 않고:

```text
127.0.0.1:31001
```

에만 Listen하도록 구성할 수 있다.

이 경우 물리 LAN이나 인터넷에서는 직접 접근할 수 없다.

```text
Internet
   X
   │
127.0.0.1:31001
   ▲
   │
MyWAN Agent
   ▲
   │
Encrypted Overlay
   │
Authorized Peer
```

따라서 다음이 필요 없어질 수 있다.

- 공유기 Port Forwarding
- 인터넷 대상 게임 Port 개방
- 실제 Backend Port 공개

---

## 20. 게임 서버와 IPv6 문제

게임 서버나 클라이언트 중에는 IPv6를 지원하지 않거나 부분적으로만 지원하는 경우가 있다.

따라서 MyWAN에서는 애플리케이션 계층과 실제 Transport 계층을 분리하는 것이 좋다.

게임에는 IPv4 VIP를 보여준다.

```text
minecraft.mywan
    ↓
100.66.0.1
```

하지만 실제 두 MyWAN Node 간 터널은 IPv6를 사용할 수도 있다.

```text
Game Client
IPv4 only
    ↓
100.66.0.1:25565
    ↓
MyWAN Client
    ↓
IPv6 UDP Direct Tunnel
    ↓
Remote MyWAN Agent
    ↓
IPv4 localhost
    ↓
Game Server
```

게임 자체는 IPv6가 사용된 사실을 알 필요가 없다.

---

## 21. IPv4와 IPv6 역할 분리

권장 구조:

```text
Application Network
────────────────────────

IPv4 VIP
게임 / RDP / DB / Legacy App 호환

100.64.x.x
100.65.x.x
100.66.x.x


Transport Network
────────────────────────

LAN IPv4
LAN IPv6
Public IPv4
Public IPv6
UDP NAT Traversal
Relay

자동 Path Selection
```

IPv6는 내부 애플리케이션 주소로 강제하기보다 실제 Node 간 P2P Transport에서 적극 활용할 수 있다.

---

## 22. 가상망 주소 대역

IPv4에서 사용할 수 있는 대표적인 비공인/특수 대역:

```text
10.0.0.0/8
172.16.0.0/12
192.168.0.0/16
100.64.0.0/10
```

단:

- `192.168.0.0/16`은 가정용 LAN과 충돌 가능성이 매우 높음
- `10.0.0.0/8`은 기업 네트워크와 충돌 가능성이 높음
- `172.16.0.0/12`도 기업 환경에서 자주 사용
- `100.64.0.0/10`은 CGNAT Shared Address Space이며 Overlay Network에 비교적 적합

예:

```text
100.64.0.0/10
MyWAN 전체 주소 공간
```

---

## 23. IPv4 주소 공간 예시

예:

```text
100.64.0.0/16
Device / Node IP

100.65.0.0/16
Infrastructure

100.66.0.0/16
Service VIP

100.67.0.0/16 ~
향후 확장
```

예:

```text
MAIN-PC
100.64.0.10

Laptop
100.64.0.11

NAS
100.64.0.12
```

서비스:

```text
Minecraft
100.66.0.1

Factorio
100.66.0.2

PostgreSQL
100.66.0.3
```

---

## 24. IPv6 ULA

장기적으로는 IPv6 ULA도 같이 사용할 수 있다.

ULA 범위:

```text
fc00::/7
```

실제 로컬 생성에는 보통:

```text
fd00::/8
```

을 사용한다.

예:

```text
fd8a:73c2:918f::/48
```

구조:

```text
fd8a:73c2:918f:1::/64
  Client Devices

fd8a:73c2:918f:2::/64
  Server Nodes

fd8a:73c2:918f:3::/64
  Infrastructure

fd8a:73c2:918f:ffff::/64
  Service VIP
```

단 게임 호환성을 고려하면 IPv4 VIP를 기본으로 하고 IPv6는 선택적으로 제공하는 것이 현실적이다.

---

## 25. Dual Stack 예시

Node:

```text
IPv4
100.64.0.10

IPv6
fd8a:73c2:918f:1::10
```

DNS:

```text
main-pc.mywan

A
100.64.0.10

AAAA
fd8a:73c2:918f:1::10
```

서비스:

```text
minecraft.mywan

A
100.66.0.1
```

게임에는 필요하다면 IPv4 A Record만 제공할 수 있다.

웹이나 일반 서비스는 A + AAAA를 제공할 수 있다.

---

## 26. IP 충돌 문제

`100.64.0.0/10`도 완전히 충돌이 없는 대역은 아니다.

일부 ISP가 실제 CGNAT에서 이 대역을 사용할 수 있다.

예:

```text
Physical Network
100.72.31.20
```

MyWAN에서도 같은 범위를 사용하면 Routing Conflict 가능성이 있다.

따라서 실제 제품에서는 다음을 고려할 필요가 있다.

- Network/Tenant별 독립 주소 공간
- IPv6 ULA 병행
- Route Conflict Detection
- Dynamic Prefix Allocation
- 필요 시 Address Translation
- Namespace 기반 Routing

---

## 27. Tenant별 독립 주소 공간

대규모 환경에서는 하나의 거대한 주소 공간을 모든 사용자가 공유할 필요가 없다.

예:

```text
Tenant A
100.64.0.10

Tenant B
100.64.0.10
```

두 네트워크의 Network ID가 다르면 논리적으로 서로 충돌하지 않도록 설계할 수 있다.

즉 Routing Key를 단순히:

```text
IP
```

로 두지 않고:

```text
Network ID + Virtual IP
```

로 관리할 수 있다.

이 구조는 SDN에 가까워진다.

---

## 28. Control Server가 Main PC에 있을 경우

개인 MVP라면 Main PC 하나에서 다음을 모두 구동할 수 있다.

```text
Main PC

MyWAN Control Server
IPAM
Internal DNS
STUN
Peer Registry
Service Registry
ACL
Relay
```

다른 PC는 설치 후 로그인만 하면 된다.

```text
설치
  ↓
로그인
  ↓
Virtual NIC 생성
  ↓
IP 자동 할당
  ↓
Split DNS 설정
  ↓
Route 설정
  ↓
MyWAN 접속 완료
```

다만 Main PC가 꺼지면 Control Plane 장애가 발생할 수 있다.

예:

```text
Main PC OFF
   ↓
신규 로그인 불가
Peer Discovery 갱신 불가
DNS 갱신 불가
Service Discovery 장애
```

이미 연결된 P2P Session은 일정 시간 유지하도록 구현할 수 있다.

실제 서비스 단계에서는 Control Plane을 별도 VPS나 서버에 두는 것이 더 안정적이다.

---

## 29. 권장 초기 기술 스택

초기 MVP에서는 다음 정도가 현실적이다.

```text
Windows Client
  ├─ Wintun
  ├─ Rust 또는 Go
  ├─ Routing Manager
  ├─ Split DNS Manager
  ├─ Service Router
  ├─ NAT Traversal
  └─ Encrypted Tunnel

Control Server
  ├─ Authentication
  ├─ Device Registry
  ├─ IPAM
  ├─ Service Registry
  ├─ ACL
  ├─ DNS
  ├─ STUN
  └─ Relay
```

Transport:

```text
WireGuard-style encrypted UDP
또는
QUIC 기반 터널
```

암호화 프로토콜은 처음부터 독자 설계하기보다 검증된 기술을 활용하는 것이 좋다.

---

## 30. 최종 사용자 경험

사용자가 느끼는 경험은 최대한 단순하게 만든다.

```text
1. 설치 프로그램 실행
2. 로그인
3. 가상 NIC 자동 생성
4. 가상 IP 자동 할당
5. Split DNS 자동 구성
6. MyWAN Route 자동 구성
7. 연결 완료
```

이후:

```text
minecraft.mywan
nas.mywan
office-pc.mywan
postgres.mywan
```

등의 이름만 사용한다.

사용자는 다음을 몰라도 된다.

- 실제 서버 Public IP
- 실제 Private IP
- 공유기 구조
- CGNAT 여부
- Port Forwarding
- 실제 Backend Port
- 서버 물리 위치
- IPv4/IPv6 Transport
- P2P/Relay 여부

Control Plane과 MyWAN Client가 이를 자동 처리한다.

---

## 31. 전체 개념 요약

가장 핵심적인 구조는 다음과 같다.

```text
                   MyWAN Control Plane

       Authentication / IPAM / DNS / ACL
             Service Registry / STUN
                       │
                       │
───────────────────────┼──────────────────────
                       │
                       ▼

                 Virtual Network

User PC
100.64.0.20
    │
    │ minecraft.mywan
    ▼
100.66.0.1
Service VIP
    │
    ▼
Control Plane Lookup
    │
    ├─ ACL
    ├─ Target Node
    ├─ Target Port
    ├─ Endpoint
    └─ Best Path
            │
      ┌─────┴─────┐
      │           │
 Direct P2P      Relay
      │
      ▼
Server Node
100.64.0.10
      │
      ▼
MyWAN Agent
      │
      ▼
127.0.0.1:31001
      │
      ▼
Game Server
```

결국 이 프로젝트의 본질은 단순한 VPN이 아니라 다음에 가깝다.

> 가상 IP 기반 Overlay Network 위에 Service Discovery, DNS, NAT Traversal, P2P Routing, ACL, Service VIP, L4 Port Mapping을 통합한 소프트웨어 정의 네트워크.

게임 서버에서는 특히 다음 구조가 핵심이다.

```text
Domain
  ↓
Service VIP
  ↓
L4 TCP/UDP Mapping
  ↓
P2P Overlay
  ↓
Actual Backend
```

이를 통해 공인 IP가 하나뿐인 환경에서도 여러 게임 서버나 서비스를 각각 독립된 가상 IP와 도메인으로 제공할 수 있다.

---
---

# 확장판 B — 오픈소스 셀프호스팅 & 다중 네트워크 클라이언트

> 위 1~31절은 "중앙 Control Plane 하나가 전체를 관리하는 SaaS형"을 전제로 한다.
> 아래 32~46절은 실제 목표에 맞게 이를 **오픈소스 셀프호스팅 모델**로 바꾼다.
>
> 새 요구사항 요약:
> 1. 서버를 각 사용자가 **직접 호스팅**한다(오픈소스). 각자 자기만의 Private 네트워크를 만든다.
> 2. 서버 호스트는 **자기 도메인을 지정**한다. 서버 호스트에게는 **공인 IP가 필요**하다.
> 3. **한 클라이언트가 여러 서버(네트워크)에 동시에 가입**할 수 있어야 한다.

---

## 32. 배포 모델 변경: 중앙 SaaS → 오픈소스 셀프호스팅

기존(1~31절)은 "MyWAN 회사가 Control Plane을 운영하고 사용자는 Client만 설치"하는 모델이었다.

새 모델은 다음과 같다.

```text
[기존] 중앙 집중형(SaaS)
      하나의 Control Plane ── 모든 사용자 공유

[신규] 셀프호스팅형(오픈소스)
      Alice의 서버 ── Alice의 네트워크(가입자 A1, A2 …)
      Bob의 서버   ── Bob의 네트워크(가입자 B1, B2 …)
      Carol의 서버 ── Carol의 네트워크(…)
      … 서로 완전히 독립. 중앙 조율자 없음.
```

즉 서버 바이너리와 클라이언트 바이너리를 **오픈소스로 배포**하고, 누구나 자기 서버를 띄우면
그 서버가 곧 하나의 독립된 Control Plane이 된다. 이는 Headscale(셀프호스팅 Tailscale 컨트롤러),
ZeroTier의 self-hosted controller, Netmaker 와 같은 계열이다.

핵심 함의:
- 전역 공유 자원(전역 relay, 전역 DNS 루트, 전역 IPAM)이 **없다**. 각 서버가 자기 것을 제공한다.
- 신뢰의 뿌리가 "중앙 회사"가 아니라 **각 서버(도메인+키)** 이다.
- 클라이언트는 여러 독립 서버에 각각 가입하므로, 네트워크마다 **분리된 신원·키·주소·DNS**를 가진다.

---

## 33. 네트워크의 정체성 = 호스트가 지정한 도메인 (매우 중요)

기존 6절은 "임의 사설 TLD(`*.mywan`)보다 실제 소유 도메인 하위를 쓰는 것도 좋다"고 *권유*만 했다.
셀프호스팅 + 다중 네트워크에서 **필수인 것은 "네트워크마다 유일한 DNS 서픽스"** 이고, **실제 도메인
보유 자체는 선택**이다(호스트에게 도메인이 없으면 서버가 유일 서픽스를 발급하고, 클라이언트가 로컬
별칭으로 보정한다 — 47절에서 갱신). 아래 도메인 사용 시나리오는 "도메인이 있는 경우"의 이상적 형태다.

서버 호스트는 네트워크를 만들 때 도메인(또는 서브도메인)을 지정한다.

```text
Alice: alice.example.com   →  네임스페이스 *.alice.example.com
Bob:   home.bob.net        →  네임스페이스 *.home.bob.net
```

이 도메인은 세 가지 역할을 **동시에** 한다.

1. Control Server 위치 탐색
   ```text
   alice.example.com  ─(A/AAAA 또는 SRV)→  공인 IP:포트  (컨트롤 서버)
   ```
2. TLS 신원 (Let's Encrypt 등 공인 CA 인증서의 CN/SAN)
   → 클라이언트가 "이 서버가 진짜 alice.example.com 인가"를 검증.
3. **DNS 검색 서픽스** = 네트워크 네임스페이스
   ```text
   minecraft.alice.example.com   (Alice 네트워크의 서비스)
   nas.home.bob.net              (Bob 네트워크의 서비스)
   ```

도메인이 전역적으로 유일하므로 **서로 다른 네트워크의 이름이 절대 충돌하지 않는다.**
이것이 "클라이언트가 여러 네트워크에 동시 가입" 문제의 절반을 이름 차원에서 해결한다.

> 도메인이 없는 사용자를 위한 대안: 프로젝트가 무료 서브도메인 서비스를 제공하거나
> (`<name>.mywan.app` 형태), 또는 도메인 없이 IP 직접 지정 + 자체 서명 인증서(TOFU 핀닝)를
> 허용한다. 다만 다중 네트워크 충돌 회피를 위해 **네트워크마다 유일한 서픽스**는 여전히 강제한다
> (예: 서버가 생성 시 랜덤 서픽스 `n-7f3a2c.mywan` 발급).

---

## 34. 서버 호스트 요구사항 (공인 IP 필수)

서버를 호스팅하려는 사람에게 필요한 것:

```text
필수
  - 공인 IP (IPv4 권장, IPv6 병행 가능). CGNAT 뒤면 호스팅 불가(또는 상위 relay 필요).
  - 지정 도메인의 A/AAAA(또는 SRV) 레코드를 이 공인 IP로.
  - 개방 포트:
      443/tcp 또는 QUIC/443/udp  : 컨트롤 API + TLS
      3478/udp                   : STUN (NAT 판별/엔드포인트 수집)
      <relay>/udp                : P2P 실패 시 릴레이 (호스트 대역폭 소모)
  - TLS 인증서: ACME 자동발급(도메인 소유 검증) 또는 자체 서명+핀닝.

권장
  - 상시 가동(VPS/홈서버). 꺼지면 신규 가입·DNS 갱신·피어 디스커버리 중단(28절 참고).
  - 방화벽에서 위 포트만 개방.
```

한 대의 호스트가 이 네트워크의 **Control Plane 전체**를 제공한다.

```text
셀프호스팅 서버 (Alice)
 ├─ Authentication / Membership      (가입자·장치 인증)
 ├─ IPAM (이 네트워크 전용 주소 공간)
 ├─ Authoritative DNS  *.alice.example.com
 ├─ Service Registry / ACL
 ├─ Peer Directory (엔드포인트 교환)
 ├─ STUN
 └─ Relay (P2P 실패 시 폴백; 공인 IP 이므로 릴레이도 가능)
```

---

## 35. 가입/온보딩 흐름 (pre-auth key / invite)

오픈소스 셀프호스팅에서 새 클라이언트가 네트워크에 들어오는 표준 흐름:

```text
관리자(호스트)                         클라이언트
─────────────                         ─────────
1. 네트워크 생성
   도메인 지정, IPAM 대역 지정
2. 초대 토큰(pre-auth key) 발급  ───►  3. join 실행
   (만료/1회용/역할 지정 가능)              app join alice.example.com --key <TOKEN>
                                         4. 노드 키쌍 생성(이 네트워크 전용)
                                         5. 서버에 등록 요청(토큰+공개키)
6. 토큰 검증 → 장치 승인               ◄──
   IP 할당, DNS 서픽스, ACL 부여
                                         7. 프로파일 저장, 가상 NIC 구성, 접속 완료
```

- 토큰은 Headscale/ZeroTier 의 pre-auth key 와 동일 개념(1회용·만료·태그 지정 가능).
- 수동 승인 모드(관리자가 대시보드에서 승인)도 옵션으로 제공.
- 재설치/재부팅 시 Device UUID + 저장된 키로 **같은 IP 재할당**(4절과 동일).

---

## 36. 신뢰 모델 (서버·클라이언트 상호 인증)

중앙 회사가 보증해 주지 않으므로 신뢰의 뿌리를 명시해야 한다.

```text
클라이언트 → 서버 검증
  A) 공인 도메인 + 공인 CA 인증서(ACME): 표준 TLS 검증. 가장 매끄럽고 안전. 권장.
  B) 도메인 없음/사설: 최초 접속 시 서버 공개키 지문(fingerprint)을 TOFU 로 고정(pin).
     초대 토큰 안에 서버 지문을 넣어 배포하면 MITM 완화.

서버 → 클라이언트 검증
  - 초대 토큰(pre-auth key)로 최초 인증.
  - 이후에는 클라이언트가 등록 시 올린 노드 공개키로 지속 인증.

데이터 평면 암호화
  - 노드 간 터널은 WireGuard 스타일 Noise 핸드셰이크(공개키 상호 인증).
  - 키는 네트워크마다 별도(다중 네트워크 시 신뢰 경계 분리).
```

원칙: **네트워크 A의 관리자는 네트워크 B의 트래픽·키·멤버를 절대 볼 수 없다.**

---

## 37. 클라이언트가 여러 네트워크에 동시 가입 — 핵심 과제

가장 어려운 신규 요구사항. 한 클라이언트가 Alice·Bob 네트워크에 동시 가입하면
다음이 전부 충돌 가능하다.

```text
충돌 지점                      원인
────────                      ────
1. 오버레이 IP 대역            둘 다 100.64.0.0/10 사용 가능
2. 개별 노드 IP               둘 다 100.64.0.10 발급 가능
3. DNS 서픽스                 (33절로 해결됨: 도메인이 유일)
4. OS 라우팅 테이블           같은 CIDR를 두 어댑터로 라우팅 불가(전역 테이블)
5. 신원/키/ACL               네트워크별로 완전히 분리되어야 함
6. STUN/Relay/피어 디렉터리   네트워크마다 다른 서버
```

Tailscale 등 기성 오버레이가 역사적으로 "한 번에 하나의 tailnet"만 지원한 이유가 바로 이
충돌 때문이다. 셀프호스팅 다중 네트워크는 이 프로젝트의 **핵심 차별점이자 핵심 난제**다.

해결 전략은 38~40절.

---

## 38. 주소 충돌 해법: 어댑터 분리 + 클라이언트-로컬 VIP 할당

핵심 아이디어 두 가지.

### (1) 네트워크마다 별도의 가상 NIC(Wintun 어댑터)
가입한 네트워크 하나당 Wintun 어댑터를 하나씩 만든다.

```text
Wintun "MyWAN-alice"  ← Alice 네트워크 전용
Wintun "MyWAN-bob"    ← Bob 네트워크 전용
```

어댑터가 분리되면 인터페이스별 라우팅·DNS(NRPT)를 개별 적용할 수 있다.

### (2) 앱에 보이는 IP는 "클라이언트-로컬 VIP"로 재할당
서로 다른 네트워크가 내부적으로 같은 숫자(`100.64.0.10`)를 써도, **클라이언트가 앱에게
보여 주는 주소는 그 클라이언트 안에서만 유일하게 로컬 배정**한다. Agent가 매핑한다.

```text
Alice 네트워크의 노드 X (오버레이 원주소 100.64.0.10)
      → 이 클라이언트에서는 로컬 VIP 100.80.0.10 으로 표현

Bob 네트워크의 노드 Y (오버레이 원주소 100.64.0.10)  ← 숫자 충돌!
      → 이 클라이언트에서는 로컬 VIP 100.90.0.10 으로 표현  ← 충돌 회피
```

매핑 테이블(클라이언트 로컬):

```text
LOCAL_VIP       NETWORK   REAL_OVERLAY_ADDR   ADAPTER
100.80.0.10     alice     100.64.0.10         MyWAN-alice
100.90.0.10     bob       100.64.0.10         MyWAN-bob
100.80.66.1     alice     svc:minecraft       MyWAN-alice
```

DNS는 이 **로컬 VIP**를 응답한다. 즉:

```text
minecraft.alice.example.com  →  100.80.66.1   (이 클라이언트 로컬)
minecraft.home.bob.net       →  100.90.66.1   (이 클라이언트 로컬)
```

> 결론: **이름(도메인)이 안정적인 계약이고, 숫자 IP는 클라이언트 내부의 임시 로컬 표현이다.**
> 12절의 "Service VIP는 실제 NIC 주소라기보다 논리적 Service ID"라는 통찰을 다중 네트워크로
> 확장한 것. 이렇게 하면 네트워크 수가 아무리 늘어도 OS 라우팅/주소 충돌이 원천 차단된다.

각 어댑터는 자기 로컬 VIP 대역(예: alice=100.80.0.0/16, bob=100.90.0.0/16)만
자신에게 라우팅하므로 전역 라우팅 테이블에서도 충돌하지 않는다.

---

## 39. Split DNS 다중 서픽스 (네트워크별 NRPT 규칙)

Windows의 NRPT(Name Resolution Policy Table)를 이용해 **서픽스별로 다른 내부 리졸버**로 보낸다.

```text
*.alice.example.com   →  127.0.0.1:53 (MyWAN 로컬 리졸버, network=alice)
*.home.bob.net        →  127.0.0.1:53 (MyWAN 로컬 리졸버, network=bob)
그 외 (google.com …)   →  기존 시스템 DNS
```

로컬 리졸버는 질의된 이름의 서픽스로 어느 네트워크인지 판별하고, 그 네트워크의
서비스/노드 레코드를 조회해 **클라이언트-로컬 VIP**(38절)를 응답한다.

- 도메인이 유일(33절)하므로 서픽스 매칭만으로 네트워크가 결정된다.
- `Add-DnsClientNrptRule -Namespace ".alice.example.com" -NameServers 127.0.0.1` 형태.
- 시스템 전체 DNS를 바꾸지 않으므로 일반 인터넷에 영향 없음(5·6절 유지).

---

## 40. 네트워크 간 격리 (라우팅 누수 방지)

한 클라이언트가 두 네트워크에 동시에 있으면, 실수로 A↔B 사이 브리지가 되어선 안 된다.

```text
금지: Alice 네트워크의 트래픽이 이 클라이언트를 거쳐 Bob 네트워크로 새는 것.
```

방지책:
- Agent 내부 포워딩 테이블을 **네트워크별로 완전 분리**(공유 라우팅 금지).
- 각 어댑터에서 들어온 패킷은 **같은 네트워크의 피어로만** 포워딩.
- IP 포워딩(라우터 역할)은 기본 off. 명시적으로 "이 노드를 subnet router로" 지정한
  경우에만, 그리고 **단일 네트워크 범위 안에서만** 허용.
- ACL은 네트워크별 Control Server가 각자 관리(36절의 신뢰 경계).

---

## 41. STUN / Relay 연합 (각 호스트가 자체 제공)

전역 공유 relay가 없으므로 NAT 트래버설 자원도 네트워크별로 제공된다.

```text
Alice 네트워크의 P2P 시도
  STUN  = alice.example.com:3478
  Relay = alice.example.com:<relay>   (Alice 호스트 대역폭 사용)

Bob 네트워크의 P2P 시도
  STUN  = home.bob.net:3478
  Relay = home.bob.net:<relay>        (Bob 호스트 대역폭 사용)
```

경로 선택(8절)은 네트워크마다 독립적으로 수행:

```text
1. Same LAN → 2. IPv6 Direct → 3. IPv4 Direct
→ 4. UDP NAT Traversal → 5. (선택)Peer Relay → 6. Server Relay
```

호스트가 공인 IP를 가지므로 Server Relay 폴백이 항상 성립(최악의 경우에도 연결 보장).
클라이언트 UDP 소켓은 네트워크별로 분리하거나, 하나의 소켓에서 세션 ID로 다중화한다.

---

## 42. Windows 클라이언트 구현 세부

```text
구성 요소
 ├─ Windows Service (SYSTEM 권한, 상시 구동)
 │    - 다중 Wintun 어댑터 생성/관리 (네트워크당 1개)
 │    - 라우트 관리 (어댑터별 로컬 VIP 대역)
 │    - NRPT 규칙 등록/해제 (서픽스별, 39절)
 │    - WireGuard 스타일 암호화 터널 다중 세션
 │    - NAT 트래버설(STUN/hole punching), 경로 선택, relay 폴백
 │    - 로컬 DNS 리졸버(127.0.0.1:53)
 │    - Service Router: L4 TCP/UDP 매핑 (로컬 VIP:포트 → 백엔드)
 │    - 프로파일 매니저(다중 네트워크 프로파일)
 └─ (선택) 트레이 UI / CLI
      - join/leave, 네트워크 목록, 상태, 서비스 목록

Windows 특이사항 / 주의점
 - Wintun: WireGuard 프로젝트의 L3 TUN. 서명된 wintun.dll 재배포 필요(드라이버 서명).
 - 설치/드라이버 등록에 관리자 권한(UAC) 필요.
 - 방화벽: 터널 소켓 허용 + 백엔드는 127.0.0.1 에만 listen(19절) 유지.
 - 절전/재개, 네트워크 로밍(Wi-Fi↔유선) 시 재핸드셰이크/엔드포인트 갱신.
 - 다중 어댑터 각각에 metric/route 지정하여 전역 테이블 충돌 회피.
```

---

## 43. 클라이언트 프로파일/설정 저장 구조

네트워크마다 독립 프로파일. 예시:

```text
%ProgramData%\MyWAN\
  profiles\
    alice.example.com\
      node.key            (이 네트워크 전용 개인키)
      config.json         (도메인, IPAM, DNS 서픽스, 로컬 VIP 대역, ACL 캐시)
      state.json          (할당 IP, 피어 캐시, 엔드포인트)
    home.bob.net\
      node.key
      config.json
      state.json
  wintun.dll
```

프로파일이 완전히 분리되어 있어 한 네트워크를 leave 해도 다른 네트워크에 영향 없음.

---

## 44. 보안 / 키 관리 요약

```text
- 네트워크별 노드 키쌍(개인키는 클라이언트 밖으로 나가지 않음).
- 데이터 평면: 검증된 WireGuard/Noise 핸드셰이크 재사용(자체 암호 설계 금지, 29절).
- Control API: TLS(공인 CA 권장) + 초대 토큰 + 노드 키 인증.
- 초대 토큰: 1회용/만료/역할. 유출 시 폐기 가능.
- 네트워크 간 키·트래픽 완전 분리(40절).
- 백엔드 localhost-only 유지로 물리망/인터넷 직접 노출 제거(19절).
```

---

## 45. 확장 개념 요약 — 다중 네트워크 클라이언트

```text
                 클라이언트 PC (사용자 1명)
        ┌───────────────────────────────────────────┐
        │  MyWAN Agent (Windows Service)            │
        │                                           │
        │  프로파일: alice.example.com    프로파일: home.bob.net
        │   ├ node.key(A)                  ├ node.key(B)
        │   ├ Wintun "MyWAN-alice"         ├ Wintun "MyWAN-bob"
        │   ├ 로컬 VIP 100.80.0.0/16       ├ 로컬 VIP 100.90.0.0/16
        │   └ NRPT *.alice.example.com     └ NRPT *.home.bob.net
        └──────────┬───────────────────────────┬────┘
                   │ 암호화 터널(키 A)           │ 암호화 터널(키 B)
                   ▼                            ▼
        alice.example.com (공인IP)       home.bob.net (공인IP)
        Alice의 Control Plane            Bob의 Control Plane
        (독립·상호 불가시)               (독립·상호 불가시)
                   │                            │
             Alice 네트워크 피어들         Bob 네트워크 피어들
```

사용자 경험(30절)은 그대로 단순하다. 다만 이제:

```text
app join alice.example.com --key <TOKEN_A>
app join home.bob.net      --key <TOKEN_B>
```

두 번 가입하면, 앱에서는 그냥 이름만 쓴다.

```text
minecraft.alice.example.com   (Alice 네트워크)
nas.home.bob.net              (Bob 네트워크)
```

어느 이름이 어느 네트워크·어느 로컬 VIP·어느 백엔드로 가는지는 Agent가 전부 처리한다.

---

## 46. 오픈소스 산출물 구성

```text
저장소(예: vlan-controller)
 ├─ server/      셀프호스팅 Control Plane 바이너리 (누구나 자기 서버로)
 ├─ client/      Windows Agent (Service + CLI/트레이)
 ├─ shared/      프로토콜·암호·와이어 포맷 공용 라이브러리
 ├─ docs/        설치/호스팅 가이드, 프로토콜 스펙
 ├─ installer/   Windows 설치 관리자(MSI/winget), wintun 재배포
 └─ LICENSE, README, CONTRIBUTING

배포 목표
 - 서버: 단일 바이너리 + 예시 설정으로 "도메인 지정 → 실행 → 네트워크 오픈".
 - 클라이언트: 설치 → join <서버주소> + 토큰/비밀번호 → 완료.
 - 누구나 fork/self-host 가능, 중앙 의존 없음.
```

---

## 47. 요구사항 갱신 — 도메인 선택제 · GUI/트레이 · 데스크톱 앱

> 이 절은 이후 확정된 요구사항으로 33~46절을 **갱신**한다. 충돌 시 이 절이 우선한다.

### 47.1 도메인은 선택이다 (33·35·39절 갱신)
서버(사설 서버)가 곧 호스팅 주체이므로, 서비스 운영자는 **도메인이 있어도 되고 없어도 된다.**
가입은 **서버 주소 + 토큰(또는 비밀번호)** 입력으로 이뤄진다.

```text
가입 입력값
  - 서버 주소 : 도메인(alice.example.com) 또는 공인 IP:포트(203.0.113.5:443)
  - 인증      : 초대 토큰(pre-auth key) 또는 관리자 설정 비밀번호
```

도메인 유무에 따른 처리:

```text
[도메인 있음]  → TLS 공인 CA(ACME) 검증, DNS 서픽스 = 그 도메인 (*.alice.example.com)
[도메인 없음]  → IP 직접 접속 + 서버 공개키 지문 TOFU 핀닝(토큰에 지문 동봉),
                DNS 서픽스 = 서버가 발급한 유일 서픽스(예: n-7f3a2c.mywan)
```

**다중 네트워크 이름 충돌 회피(핵심):** 도메인이 없어 서픽스가 겹칠 수 있으므로,
클라이언트는 가입 시 각 네트워크에 **로컬 별칭(짧은 이름)** 을 부여하고 로컬 DNS 서픽스로 쓴다.

```text
가입: 서버 203.0.113.5:443, 토큰 …, 로컬이름 "alice"
      → 로컬에서 *.alice 로 접근 (minecraft.alice)
가입: 서버 198.51.100.9:443, 토큰 …, 로컬이름 "bob"
      → 로컬에서 *.bob 로 접근 (nas.bob)
```

즉 38절의 "클라이언트-로컬 VIP(숫자)" 원칙을 **이름(서픽스)에도 동일 적용**한다.
도메인이 있으면 그 도메인을, 없으면 로컬 별칭을 서픽스로 쓰므로 도메인 없이도 다중 가입이 안전하다.

### 47.2 서버·클라이언트 모두 "데스크톱 앱 + 트레이 백그라운드" (34·42·46절 갱신)
서버도 헤드리스 바이너리로만 두지 않고, **애플리케이션 형태로 실행**되며 **설정을 GUI로** 하고
**트레이에서 백그라운드 상주**해야 한다. 클라이언트도 동일.

권장 구조: **엔진(데몬/서비스) + 프론트엔드(GUI/트레이) 분리** (Tailscale 방식).

```text
[서버 측]  mywan-server
  ├─ 엔진(백그라운드): Control Plane 전체(인증/IPAM/DNS/레지스트리/ACL/STUN/Relay)
  │     - Windows: 서비스 또는 트레이 상주 프로세스로 구동
  │     - Linux VPS: 동일 바이너리를 헤드리스로 구동(선택)
  ├─ 트레이: 실행/중지, 상태(가입자 수·릴레이 트래픽), 로그 열기
  └─ GUI 설정창:
        - 네트워크 생성/편집(오버레이 대역, 서픽스/도메인)
        - 초대 토큰 발급·폐기, 비밀번호 설정
        - 멤버/장치 승인·조회, 서비스·ACL 관리
        - 포트/인증서(ACME 또는 자체서명) 설정

[클라이언트 측]  mywan-agent
  ├─ 엔진(백그라운드 서비스): 가상 NIC·터널·라우팅·DNS·서비스 라우터(42절)
  ├─ 트레이: 네트워크 목록/연결상태, 빠른 연결·해제
  └─ GUI 설정창:
        - 가입 대화상자(서버 주소 + 토큰/비밀번호 + 로컬이름)
        - 가입 네트워크 목록, 서비스 목록, 경로(P2P/Relay) 상태
        - 프로파일 관리(다중 네트워크)
```

프론트엔드는 로컬 IPC/API로 엔진과 통신한다(엔진은 백그라운드 상주, GUI는 필요 시 열림).

### 47.3 반영 요약
```text
33절: 도메인 "필수" → "유일 서픽스는 필수, 도메인은 선택"
34절: 서버 = 헤드리스 전용 → 데스크톱 앱(GUI+트레이) + 헤드리스(선택)
35절: join <domain> --key → 서버주소(도메인/IP) + 토큰/비밀번호 + 로컬이름
39절: DNS 서픽스 = 도메인 → 도메인 있으면 도메인, 없으면 로컬 별칭 서픽스
42·46절: 클라·산출물에 GUI 설정창 + 트레이 백그라운드 명시
```
