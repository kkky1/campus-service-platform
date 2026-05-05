# Campus Service Platform

面向高校学生的校园服务平台，提供校园店铺查询、活动报名、校园福利券抢购、学生动态分享、关注互动、签到积分等功能。项目围绕校园生活服务场景重构前端展示文案，并保留后端高并发优惠券、Redis 缓存、登录态管理、签到统计等核心能力。

## 技术栈

- 后端：Spring Boot、Spring MVC、MyBatis-Plus、MySQL
- 缓存与并发：Redis、Lua、Redisson、分布式锁、RabbitMQ
- 前端：Vue、Element UI、Axios、Nginx 静态资源服务
- 本地部署：Docker Compose、Nginx 反向代理、Maven

## 核心功能

- 手机验证码登录：使用 Redis 存储登录 Token，替代传统 Session，支持多实例部署下的登录态共享。
- 双拦截器认证：将 Token 刷新与登录校验拆分，提升认证逻辑的复用性和可维护性。
- 校园服务查询：支持校园服务分类、店铺列表、店铺详情、服务券展示等业务页面。
- 热点数据缓存：使用 Redis 缓存高频访问数据，降低数据库查询压力。
- 校园福利券抢购：使用 Redis + Lua 将库存校验、重复抢券校验、扣减库存、记录用户行为封装为原子操作。
- 异步下单削峰：秒杀请求通过 Lua 完成资格校验后写入 RabbitMQ，由消费者异步创建订单，降低主链路响应时间。
- 防重复参与：结合 Redis、Lua、数据库唯一约束和分布式锁，避免高并发下重复报名、重复抢券和库存超卖。
- 签到与统计：使用 Redis Bitmap 实现用户签到统计，使用 HyperLogLog 进行 UV 统计。
- 社交互动：支持学生动态发布、点赞、关注和共同关注查看。

## 项目结构

```text
.
├── front/                         # 前端静态页面与 Nginx 资源
│   └── html/hmdp/                 # 校园服务平台页面
├── src/main/java/com/hmdp/        # Spring Boot 后端代码
├── src/main/resources/            # 配置、Mapper、Lua 脚本与数据库脚本
├── docker-compose.local.yml       # MySQL、Redis、RabbitMQ、Nginx 本地环境
├── nginx.local.conf               # Nginx 本地反向代理配置
└── pom.xml                        # Maven 项目配置
```

## 本地运行

1. 启动基础服务：

```bash
docker compose -f docker-compose.local.yml up -d
```

2. 初始化数据库：

```bash
mysql -h 127.0.0.1 -P 3306 -uroot -p123456 hmdp < src/main/resources/db/hmdp.sql
```

3. 启动后端服务：

```bash
mvn spring-boot:run -Dspring-boot.run.profiles=local
```

也可以先打包后运行：

```bash
mvn clean package -DskipTests
java -jar target/*.jar --spring.profiles.active=local
```

4. 访问前端页面：

```text
http://localhost:8080/
```

后端接口地址：

```text
http://localhost:8081/
```

RabbitMQ 管理台：

```text
http://localhost:15672/
```

默认账号密码为 `guest / guest`。

## 简历亮点表达

- 基于 Spring Boot + Redis + RabbitMQ 实现面向高校学生的校园服务平台，覆盖校园店铺查询、活动报名、福利券抢购、学生动态分享、关注互动、签到积分等业务场景。
- 在高并发抢券场景中使用 Redis + Lua 将库存校验、重复参与校验、扣减库存和用户行为记录封装为原子操作，减少数据库压力并避免超卖问题。
- 秒杀请求通过 Lua 完成资格校验后写入 RabbitMQ，由消费者异步创建订单，实现削峰填谷，降低接口响应时间并提升吞吐能力。
- 使用 Redis 存储用户登录态，替代传统 Session，并通过双拦截器拆分 Token 刷新与登录校验逻辑，提升认证模块的可维护性。
- 使用 Redis Bitmap 实现连续签到统计，使用 HyperLogLog 统计 UV，体现对 Redis 高级数据结构的实际应用能力。

## 说明

该项目由本地生活服务场景改造为校园服务平台主题。为控制改造范围，后端包名、数据库名和部分历史资源目录仍保留原始工程命名，不影响项目运行和业务展示。
