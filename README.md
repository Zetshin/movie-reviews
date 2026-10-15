# เว็บแอปพลิเคชันรีวิวภาพยนตร์ (MovieClub TH)

[![CI](https://github.com/Zetshin/movie-reviews/actions/workflows/ci.yml/badge.svg)](https://github.com/Zetshin/movie-reviews/actions/workflows/ci.yml)

เว็บไซต์: <https://movieclubth.com>

เว็บรีวิวภาพยนตร์เขียนด้วย Go และ MySQL/MariaDB เรนเดอร์ HTML ฝั่งเซิร์ฟเวอร์ ดึงข้อมูลภาพยนตร์และนักแสดงจาก TMDB ผู้ใช้สมัครสมาชิก เขียน/แก้ไขรีวิว และอัปโหลดรูปโปรไฟล์ได้ ผู้ดูแลระบบจัดการคลังภาพยนตร์และนักแสดงได้

## สารบัญ

- [ฟีเจอร์](#features)
- [เทคโนโลยีที่ใช้](#tech-stack)
- [โครงสร้างโปรเจกต์](#project-structure)
- [เริ่มใช้งานด้วย Docker (แนะนำ)](#docker)
- [รันในเครื่องโดยไม่ใช้ Docker](#local)
- [การตั้งค่า](#configuration)
- [Deploy ขึ้นเซิร์ฟเวอร์ (HestiaCP)](#deploy)
- [CI/CD](#ci-cd)
- [ความปลอดภัย](#security)
- [เส้นทาง HTTP](#http-routes)
- [ฐานข้อมูล](#database)
- [ข้อจำกัดปัจจุบัน](#known-limitations)

<a id="features"></a>

## ฟีเจอร์

- หน้าแรกแสดง Hero จากภาพยนตร์ยอดนิยมบน TMDB, ภาพยนตร์ใหม่, Top Picks พร้อมตัวอย่างรีวิว และนักแสดงที่กำลังมาแรง
- รายการภาพยนตร์และนักแสดงแบบแบ่งหน้า ค้นหาแบบ dropdown (`/movies/search`, `/actors/search`)
- หน้ารายละเอียดภาพยนตร์พร้อมนักแสดง รีวิวแบ่งหน้าละ 10 รายการ เรียงล่าสุดหรือ `?sort=popular`
- สมัครสมาชิก / เข้าสู่ระบบ / ออกจากระบบ / รีเซ็ตรหัสผ่านทางอีเมล (bcrypt) เขียนและแก้ไขรีวิว (0.5–5 ดาว, ซ่อนสปอยล์ได้) อัปโหลดรูปโปรไฟล์
- ผู้ดูแลระบบ: นำเข้าภาพยนตร์/นักแสดงจาก TMDB เพิ่ม แก้ไข ลบ และอัปโหลดโปสเตอร์/ภาพพื้นหลัง
- คะแนนเฉลี่ยและจำนวนรีวิวคำนวณใหม่ใน transaction เดียวกับการเขียนรีวิว

<a id="tech-stack"></a>

## เทคโนโลยีที่ใช้

| ส่วนประกอบ | เทคโนโลยี |
|------------|-----------|
| ภาษา | Go 1.27 (toolchain `go1.27.2`) |
| เว็บเซิร์ฟเวอร์ | `net/http`, `github.com/justinas/alice` |
| ฐานข้อมูล | MySQL 8 หรือ MariaDB 10.6+ ผ่าน `database/sql` + `go-sql-driver/mysql`, migration ฝังในไบนารี |
| เซสชัน / CSRF | `alexedwards/scs` (เก็บใน MySQL), `justinas/nosurf` |
| ข้อมูลภาพยนตร์ | TMDB API |
| คอนเทนเนอร์ | Docker (distroless, non-root), Docker Compose |
| Production | HestiaCP (nginx + Let's Encrypt) อยู่หลัง Cloudflare, MariaDB ของเซิร์ฟเวอร์ |
| CI/CD | GitHub Actions, GHCR |

<a id="project-structure"></a>

## โครงสร้างโปรเจกต์

```text
movie-reviews/
|-- cmd/web/                  # main, routes, handlers, middleware, templates
|-- internal/
|   |-- models/               # SQL ของแต่ละตาราง
|   |   `-- migrations/       # สคีมาฐานข้อมูล (ฝังในไบนารีด้วย go:embed)
|   |-- tmdb/                 # TMDB API client
|   `-- validator/            # ตรวจสอบข้อมูลฟอร์ม
|-- ui/html, ui/static/       # เทมเพลตและไฟล์ static
|-- deploy/hestia/            # production บน HestiaCP: proxy template, compose, สคริปต์
|-- .github/workflows/        # ci.yml, deploy.yml
|-- Dockerfile
|-- docker-compose.yml        # app + MySQL (ใช้ในเครื่อง)
`-- .env.example
```

<a id="docker"></a>

## เริ่มใช้งานด้วย Docker (แนะนำ)

ต้องมี Docker 24+ และ TMDB API key

```sh
git clone https://github.com/Zetshin/movie-reviews.git
cd movie-reviews
cp .env.example .env          # ใส่ TMDB_API_KEY และตั้งรหัสผ่าน MySQL
docker compose up -d --build
```

เปิด <http://localhost:4000> ระบบสร้างตารางให้อัตโนมัติตอนเริ่ม (`DB_MIGRATE=true`) ข้อมูล MySQL และไฟล์อัปโหลดเก็บใน volume `db-data` และ `uploads`

นำเข้าข้อมูลตั้งต้นจาก TMDB:

```sh
docker compose run --rm app -import-tmdb 20 -import-person-details -import-trending-people 12
```

ตั้งผู้ใช้เป็นผู้ดูแลระบบ (สมัครสมาชิกผ่านหน้าเว็บก่อน):

```sh
docker compose exec db sh -c 'mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" \
  -e "UPDATE users SET is_admin = TRUE WHERE email = '\''you@example.com'\''"'
```

คำสั่งอื่นที่ใช้บ่อย: `docker compose logs -f app`, `docker compose down` (เก็บข้อมูลไว้), `docker compose down -v` (ลบข้อมูลทั้งหมด)

<a id="local"></a>

## รันในเครื่องโดยไม่ใช้ Docker

ต้องมี Go ตาม `go.mod` และ MySQL 8 ที่สร้างฐานข้อมูล `movies` กับผู้ใช้ไว้แล้ว

```sh
go mod download
# HTTPS ในเครื่อง: สร้างใบรับรอง self-signed ใน ./tls
mkdir -p tls && (cd tls && go run "$(go env GOROOT)/src/crypto/tls/generate_cert.go" -host "localhost,127.0.0.1,::1")

export TMDB_API_KEY=...
go run ./cmd/web -migrate -dsn "movie_app:password@tcp(localhost:3306)/movies"
```

เปิด <https://localhost:4000> หรือถ้าไม่อยากใช้ HTTPS ให้เพิ่ม `-tls=false -secure-cookie=false`

ทดสอบ: `go test ./...`

<a id="configuration"></a>

## การตั้งค่า

ทุกตัวเลือกตั้งได้ทั้งผ่าน flag และ environment variable (flag มีลำดับความสำคัญสูงกว่า) โปรแกรมอ่าน `.env` ในโฟลเดอร์ที่รันด้วย

| Flag | Env | ค่าเริ่มต้น | คำอธิบาย |
|------|-----|-------------|----------|
| — | `TMDB_API_KEY` | (จำเป็น) | API key ของ TMDB |
| `-dsn` | `DB_DSN` | `user:pass@/movies` | สตริงเชื่อมต่อ MySQL/MariaDB (TCP หรือ `unix(/path/mysqld.sock)`) โปรแกรมเติม `parseTime=true` และ `time_zone='+00:00'` ให้เอง |
| — | `TZ` | ของระบบ | เขตเวลาที่ใช้แสดงวันที่บนหน้าเว็บ เช่น `Asia/Bangkok` (ในฐานข้อมูลเก็บเป็น UTC เสมอ) |
| `-addr` | `ADDR` | `:4000` | ที่อยู่ที่เซิร์ฟเวอร์รับการเชื่อมต่อ |
| `-tls` | `TLS_ENABLED` | `true` | ให้บริการ HTTPS เอง (ใน Docker เป็น `false` เพราะ reverse proxy ทำ TLS ให้) |
| `-tls-cert`, `-tls-key` | `TLS_CERT_FILE`, `TLS_KEY_FILE` | `./tls/cert.pem`, `./tls/key.pem` | ไฟล์ใบรับรอง |
| `-secure-cookie` | `SECURE_COOKIE` | `true` | ตั้งคุกกี้เป็น `Secure` และส่ง HSTS ปิดได้เฉพาะตอนทดสอบผ่าน HTTP |
| `-trust-proxy` | `TRUST_PROXY` | `false` | เชื่อ `X-Real-IP` / `X-Forwarded-For` / `X-Forwarded-Proto` เปิดเฉพาะเมื่ออยู่หลัง reverse proxy ที่ไว้ใจได้ |
| `-migrate` | `DB_MIGRATE` | `false` | สร้าง/อัปเดตตารางก่อนเริ่มเซิร์ฟเวอร์ |
| `-healthcheck` | — | — | เรียก `/healthz` ของตัวเองแล้วจบ (ใช้กับ Docker HEALTHCHECK) |
| `-public-base-url` | `PUBLIC_BASE_URL` | `http://localhost:4000` | URL หลักที่ใช้สร้างลิงก์รีเซ็ตรหัสผ่าน Production ต้องเป็น `https://movieclubth.com` |
| `-smtp-host`, `-smtp-port` | `SMTP_HOST`, `SMTP_PORT` | ว่าง, `587` | SMTP สำหรับส่งอีเมลรีเซ็ตรหัสผ่าน หาก host ว่างระบบจะไม่ส่งอีเมล |
| `-smtp-username`, `-smtp-password` | `SMTP_USERNAME`, `SMTP_PASSWORD` | ว่าง | บัญชี SMTP เก็บไว้ใน `.env` เท่านั้น |
| `-mail-from` | `MAIL_FROM` | ว่าง | อีเมลผู้ส่ง เช่น `no-reply@movieclubth.com` |
| `-smtp-require-tls` | `SMTP_REQUIRE_TLS` | `true` | บังคับ SMTP ให้ใช้ STARTTLS (ถ้าตั้ง username/password ต้องเป็น `true`) |

### ลืมรหัสผ่าน

- **หน้าใช้งาน:** `/user/forgot-password` (มีลิงก์จากหน้า login) และ `/user/reset-password?token=...`
- **ลิงก์รีเซ็ต:** ใช้ได้ 30 นาทีและใช้ได้ครั้งเดียว ในฐานข้อมูลเก็บเฉพาะ hash ของ token เมื่อรีเซ็ตสำเร็จ session เก่าทุกอันของบัญชีนั้นจะหลุด
- **อีเมล:** ภาษาไทยและอังกฤษ ส่งไปที่อีเมลที่บันทึกในบัญชี ส่งเบื้องหลังจึงไม่ทำให้หน้าเว็บช้า และใช้เวลาตอบเท่ากันไม่ว่าอีเมลจะมีในระบบหรือไม่
- **จำกัดการส่ง:** 3 ฉบับ/ชั่วโมง/อีเมล ร่วมกับ 10 คำขอ/นาที/IP และส่งพร้อมกันได้สูงสุด 4 ฉบับ
- **ถ้าไม่ได้ตั้ง SMTP:** ระบบไม่สร้าง token และแสดงข้อความเดิม log ของแอปจะบอกว่าส่งสำเร็จ (`password reset email sent`) หรือมีข้อผิดพลาด
- **Production:** ใช้ `mail.movieclubth.com:587` (STARTTLS) บัญชี `no-reply@movieclubth.com` ของ Hestia โดเมนมี SPF, DKIM และ DMARC แล้ว

คำสั่งนำเข้าข้อมูล (ทำงานเสร็จแล้วจบโปรแกรม): `-import-tmdb N`, `-import-person-details`, `-import-trending-people N`, `-sync-tmdb-metadata`, `-migrate-catalog-metadata`

<a id="deploy"></a>

## Deploy ขึ้นเซิร์ฟเวอร์ (HestiaCP)

Production รันอยู่ที่ <https://movieclubth.com> บนเซิร์ฟเวอร์ HestiaCP โครงสร้างคือ

```text
ผู้ใช้ → Cloudflare → nginx ของ Hestia (TLS, Let's Encrypt, proxy template "docker-movieclub")
       → 127.0.0.1:4010 → คอนเทนเนอร์ movieclub-app → MariaDB ของเซิร์ฟเวอร์ (ผ่าน Unix socket)
```

- แอปรันเป็นคอนเทนเนอร์เดียว (`deploy/hestia/docker-compose.yml`) เปิดพอร์ตแค่ loopback และใช้ฐานข้อมูลที่สร้างใน Hestia ผ่าน socket `/run/mysqld/mysqld.sock` จึงไม่ต้องเปิดพอร์ตฐานข้อมูล
- Proxy template `docker-movieclub` (`deploy/hestia/*.tpl|.stpl`) ส่งต่อไปที่แอป และบันทึก access log กับ `.bytes` ระดับ server ทำให้แผง Hestia แสดง **Bandwidth** และ AWStats ของโดเมนได้ (template ปกติของ Hestia บันทึกเฉพาะไฟล์ static แอปที่ proxy จึงขึ้น 0 MB เสมอ) Hestia จะรวม bandwidth ทุกคืนด้วย cron `v-update-sys-queue traffic`

### เตรียมเซิร์ฟเวอร์ (ครั้งเดียว)

1. ใน Hestia: เพิ่ม web domain และเปิด SSL (Let's Encrypt) แล้วสร้างฐานข้อมูล MySQL ของผู้ใช้เจ้าของโดเมน
2. คัดลอกไฟล์ใน `deploy/hestia/` ไปที่ `/opt/movie-reviews/` (`install.sh` และไฟล์ template ไว้ใน `/opt/movie-reviews/hestia/`)
3. สร้าง `/opt/movie-reviews/.env` จาก `deploy/hestia/.env.example` ตั้งค่าฐานข้อมูลและ SMTP แล้ว `chmod 600`
4. ติดตั้ง proxy template และเปิดสถิติ: `bash /opt/movie-reviews/hestia/install.sh <hestia-user> <domain>`
5. เริ่มแอป: `cd /opt/movie-reviews && sh deploy.sh ghcr.io/zetshin/movie-reviews:latest`

ตารางจะถูกสร้างหรืออัปเดตให้อัตโนมัติตอนเริ่ม ถ้าเป็นฐานข้อมูลเดิมที่สร้างตาม README เวอร์ชันเก่า migration `0002` จะเติมคอลัมน์และ index ที่ขาดให้ และ migration `0003` จะเพิ่ม token รีเซ็ตรหัสผ่านกับเวอร์ชัน session

### ตั้งค่า GitHub (Settings → Secrets and variables → Actions)

| ประเภท | ชื่อ | ค่า |
|--------|------|-----|
| Secret | `DEPLOY_HOST` | IP/โดเมนของเซิร์ฟเวอร์ |
| Secret | `DEPLOY_USER` | ผู้ใช้ SSH ที่สั่ง `docker` ได้ |
| Secret | `DEPLOY_SSH_KEY` | private key ที่ใช้ deploy เท่านั้น (ed25519) |
| Secret | `DEPLOY_KNOWN_HOSTS` | ผลลัพธ์ของ `ssh-keyscan -t ed25519 <host>` |
| Secret | `DEPLOY_PORT` | (ไม่บังคับ) พอร์ต SSH ค่าเริ่มต้น 22 |
| Variable | `DEPLOY_PATH` | (ไม่บังคับ) ค่าเริ่มต้น `/opt/movie-reviews` |
| Variable | `APP_URL` | (ไม่บังคับ) เช่น `https://movieclubth.com` ใช้ตรวจ `/healthz` หลัง deploy |

ถ้า package บน GHCR เป็น private เซิร์ฟเวอร์จะ login ด้วย token ชั่วคราวของ workflow ให้เอง

Deploy ด้วยมือบนเซิร์ฟเวอร์: `cd /opt/movie-reviews && sh deploy.sh ghcr.io/zetshin/movie-reviews:sha-<commit>`

ตั้งผู้ดูแลระบบ: สมัครสมาชิกผ่านหน้าเว็บ แล้วรัน `UPDATE users SET is_admin = TRUE WHERE email = '...'` ในฐานข้อมูล (ผ่าน phpMyAdmin ของ Hestia หรือ `mariadb`)

นำเข้าข้อมูลจาก TMDB บน production:

```sh
cd /opt/movie-reviews && docker compose run --rm app -import-tmdb 20 -import-person-details -import-trending-people 12
```

<a id="ci-cd"></a>

## CI/CD

- **CI** (`.github/workflows/ci.yml`) ทำงานทุก push ไป `main` และทุก pull request: `gofmt`, `go mod tidy`, `go vet`, `staticcheck`, `go test -race`, `govulncheck` จากนั้น build Docker image รัน compose stack จริงกับ MySQL ทดสอบ route หลักและ migration ตรวจไฟล์ compose ของ Hestia และสแกน image ด้วย Trivy
- **Deploy** (`.github/workflows/deploy.yml`) ทำงานเมื่อ CI ผ่านบน `main` (หรือกดรันเองได้): build commit ที่ผ่าน CI → push ไป `ghcr.io/<owner>/movie-reviews:sha-<commit>` และ `:latest` → SSH เข้าเซิร์ฟเวอร์ → `deploy.sh` ดึง image ใหม่ รอจน healthy และ **rollback อัตโนมัติ** ถ้า image ใหม่ไม่ healthy ถ้ายังไม่ได้ตั้ง secrets ขั้น deploy จะหยุดพร้อมบอกว่าขาดตัวไหน
- **Dependabot** อัปเดต Go modules, base images และ actions ทุกสัปดาห์

<a id="security"></a>

## ความปลอดภัย

- รหัสผ่าน bcrypt (cost 12), ป้องกัน user enumeration ด้วยการใช้เวลาเท่ากันเมื่อไม่พบอีเมล
- CSRF token ทุก POST + ตรวจ Origin, คุกกี้ `HttpOnly`, `Secure`, `SameSite=Lax`, เปลี่ยน session token ตอน login/logout
- จำกัด POST ของ login/signup 10 ครั้ง/นาที/IP, จำกัดขนาด body (ฟอร์ม 1 MB, อัปโหลด 3–12 MB)
- รูปที่อัปโหลดถูก decode และ encode ใหม่ ตั้งชื่อไฟล์แบบสุ่ม ปิดการแสดงรายการไฟล์ในโฟลเดอร์ static
- Headers: CSP (`frame-ancestors 'none'`, `object-src 'none'`, `form-action 'self'`), HSTS, `nosniff`, `X-Frame-Options`, COOP, Permissions-Policy
- คอนเทนเนอร์ distroless รันด้วยผู้ใช้ non-root, filesystem read-only, drop capabilities ทั้งหมด, แอปเปิดพอร์ตแค่ loopback และฐานข้อมูลไม่เปิดพอร์ตออกภายนอก
- IP ของผู้ใช้จริงมาจาก `X-Real-IP` ที่ nginx ตั้งหลังแปลง `CF-Connecting-IP` ของ Cloudflare แล้ว จึงจำกัด login ต่อคนได้ถูกต้อง

<a id="http-routes"></a>

## เส้นทาง HTTP

| เมธอด | เส้นทาง | สิทธิ์ | คำอธิบาย |
|-------|---------|--------|----------|
| GET | `/` | ทุกคน | หน้าแรก |
| GET | `/healthz` | ทุกคน | health check (ตรวจการเชื่อมต่อฐานข้อมูล) |
| GET | `/movies/`, `/movies/{id}` | ทุกคน | รายการและรายละเอียดภาพยนตร์ |
| GET | `/movies/search?q=`, `/actors/search?q=` | ทุกคน | ค้นหา (JSON) |
| GET | `/actors/`, `/actors/{id}` | ทุกคน | รายการและรายละเอียดนักแสดง (`/persons/*` redirect มาที่นี่) |
| GET/POST | `/user/signup`, `/user/login` | ทุกคน | สมัครสมาชิก / เข้าสู่ระบบ (POST ถูกจำกัดอัตรา) |
| POST | `/user/logout` | สมาชิก | ออกจากระบบ |
| GET | `/profile`, POST `/profile/avatar` | สมาชิก | โปรไฟล์และรูปโปรไฟล์ |
| GET/POST | `/movies/{id}/review`, `/reviews/{id}/edit` | สมาชิก | เขียน/แก้ไขรีวิวของตัวเอง |
| GET/POST | `/movies/add`, `/movies/{id}/edit`, `/movies/{id}/delete`, `/movies/import` | ผู้ดูแล | จัดการภาพยนตร์ |
| GET/POST | `/actors/add`, `/actors/{id}/edit`, `/actors/{id}/delete` | ผู้ดูแล | จัดการนักแสดง |
| GET | `/static/*` | ทุกคน | ไฟล์ static และไฟล์อัปโหลด |

<a id="database"></a>

## ฐานข้อมูล

สคีมาอยู่ใน [`internal/models/migrations`](./internal/models/migrations) และถูกฝังในไบนารี รันด้วย `-migrate` (หรือ `DB_MIGRATE=true`) ระบบบันทึกเวอร์ชันที่รันแล้วในตาราง `schema_migrations` และใช้ advisory lock (`GET_LOCK`) ป้องกันการรันซ้อน

| ไฟล์ | หน้าที่ |
|------|---------|
| `0001_initial_schema.sql` | สร้างตารางทั้งหมด (`CREATE TABLE IF NOT EXISTS`) |
| `0002_backfill_columns.sql` | อัปเกรดฐานข้อมูลที่สร้างตาม README เวอร์ชันเก่า: เติมคอลัมน์ที่ขาด ผ่อนคอลัมน์ `NOT NULL` ที่ทำให้นำเข้า TMDB ไม่ได้ เปลี่ยน rating เป็นทศนิยม และสร้าง unique key ของ `tmdb_id` |

เพิ่ม migration ใหม่ได้โดยสร้างไฟล์ `000N_description.sql` ไฟล์จะรันตามลำดับชื่อ ใช้ `ADD COLUMN IF NOT EXISTS` และ `CREATE [UNIQUE] INDEX IF NOT EXISTS` ได้ ตัว migrator จะจำลองให้ทำงานบน MySQL 8 ด้วย (ตามปกติรองรับเฉพาะ MariaDB)

เวลาทั้งหมดเก็บเป็น UTC เพราะทุก connection ตั้ง `time_zone='+00:00'` แม้เซิร์ฟเวอร์ฐานข้อมูลจะตั้งเขตเวลาอื่น (เช่น +07)

ไฟล์อ้างอิงการออกแบบอยู่ใน `docs/` (อาจไม่ตรงกับสคีมาปัจจุบันทุกคอลัมน์ ให้ยึด migration เป็นหลัก)

![แผนภาพ EER](./docs/EER%20Diagram.png)

![แผนภาพ ER ระดับแนวคิด](./docs/conceptual-ER.jpg)

ไฟล์ต้นฉบับที่แก้ไขได้: [conceptual-ER .drawio](./docs/conceptual-ER%20.drawio)

<a id="known-limitations"></a>

## ข้อจำกัดปัจจุบัน

- ยังไม่มีเส้นทางสำหรับกดไลก์รีวิว (ตาราง `review_likes` และการนับไลก์มีแล้ว) และยังลบรีวิวผ่านหน้าเว็บไม่ได้
- ตัวจำกัดอัตรา login เก็บในหน่วยความจำ ถ้ารันหลาย replica จะนับแยกกันในแต่ละ instance
- ไฟล์อัปโหลดเก็บใน volume ของเซิร์ฟเวอร์เดียว ถ้าจะขยายหลายเครื่องควรย้ายไป object storage
- ข้อผิดพลาดตอบกลับเป็นข้อความ HTTP มาตรฐาน ยังไม่มีหน้า error ที่ออกแบบเฉพาะ
- Test อัตโนมัติส่วนใหญ่ยังเป็น unit test ของ middleware/validator/migration การทดสอบกับฐานข้อมูลจริงอยู่ใน smoke test ของ CI ยังไม่มี integration test ของ model แยกเฉพาะ

## ผู้พัฒนา

[Zetshin](https://github.com/Zetshin)
