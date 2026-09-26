// 校园服务平台 · Jenkins 自动化部署流水线
// 触发：SCM 轮询 main（每 2 分钟，等价原 systemd 定时拉取）
// 说明：Jenkins 以容器运行，挂载宿主机 Docker/Go/密钥；部署使用固定 compose 项目名
//       campus-service-platform 以便接管既有容器。
pipeline {
    agent any
    options {
        timeout(time: 30, unit: 'MINUTES')
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }
    environment {
        PATH        = "/usr/local/go/bin:${env.PATH}"
        GOPROXY     = "https://goproxy.cn,direct"
        GOMODCACHE  = "/root/go/pkg/mod"
        GOCACHE     = "/root/go/build-cache"
        COMPOSE     = "docker compose -p campus-service-platform -f docker-compose.local.yml"
    }
    stages {
        stage('检出') {
            steps {
                checkout scm
                sh 'echo "当前提交: $(git log --oneline -1)"'
            }
        }
        stage('准备密钥') {
            steps {
                // 部署密钥来自服务器 .env（不入库），Jenkins 容器只读挂载
                sh 'cp /run/secrets/app.env .env && chmod 600 .env'
            }
        }
        stage('构建与测试') {
            steps {
                sh '''
                    set -a; . ./.env; set +a
                    export TEST_MYSQL_DSN="campus:${MYSQL_APP_PASSWORD}@tcp(mysql:3306)/hmdp?charset=utf8mb4&parseTime=True&loc=Local"
                    export TEST_REDIS_ADDR="redis:6379"
                    export TEST_REDIS_PASSWORD="${REDIS_PASSWORD}"
                    go build ./...
                    go vet ./...
                    go test ./...
                '''
            }
        }
        stage('编译二进制') {
            steps {
                sh 'mkdir -p bin && go build -o bin/campus-server ./cmd/server'
            }
        }
        stage('部署') {
            steps {
                sh '''
                    set -a; . ./.env; set +a
                    # 上传卷属主（容器以 uid 10001 运行）
                    docker run --rm -v hmdp-upload-data:/data alpine:3.20 chown -R 10001:10001 /data 2>/dev/null || true
                    # 首次导入数据库
                    if ! docker exec hmdp-mysql mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" hmdp -N \
                         -e 'SHOW TABLES LIKE "tb_user"' 2>/dev/null | grep -q tb_user; then
                        echo "首次部署：导入 db/hmdp.sql"
                        (echo "SET SESSION sql_mode='NO_ENGINE_SUBSTITUTION';"; cat db/hmdp.sql) \
                            | docker exec -i hmdp-mysql mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" hmdp
                    fi
                    ${COMPOSE} up -d --build --remove-orphans
                    docker image prune -f > /dev/null
                '''
            }
        }
        stage('健康检查') {
            steps {
                sh '''
                    for i in $(seq 1 30); do
                        if curl -sf http://backend:8081/shop-type/list > /dev/null; then
                            echo "✅ 健康检查通过（第 ${i} 次探测）"
                            exit 0
                        fi
                        sleep 2
                    done
                    echo "❌ 健康检查超时"
                    exit 1
                '''
            }
        }
    }
    post {
        failure {
            sh '${COMPOSE} logs backend --tail 50 || true'
        }
        success {
            echo "部署成功：${env.BUILD_NUMBER}"
        }
    }
}
