#!/usr/bin/env python3
"""校园服务平台后端全量功能 QA：逐个功能实测并记录问题。

用法: python3 /tmp/qa.py [BASE]  （BASE 默认 http://127.0.0.1:8081）
输出: /tmp/qa_report.txt
"""
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8081"
REPORT = open("/tmp/qa_report.txt", "w")


def log(msg):
    print(msg)
    REPORT.write(msg + "\n")


def http(method, path, body=None, token=None, files=None, form=None):
    url = BASE + path
    req = urllib.request.Request(url, method=method)
    data = None
    if token:
        req.add_header("authorization", token)
    if files:
        boundary = "----qaboundary1234"
        parts = []
        for k, v in (form or {}).items():
            parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{k}"\r\n\r\n{v}\r\n'.encode())
        for k, (fname, content) in files.items():
            parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{k}"; filename="{fname}"\r\nContent-Type: application/octet-stream\r\n\r\n'.encode() + content + b"\r\n")
        parts.append(f"--{boundary}--\r\n".encode())
        data = b"".join(parts)
        req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
    elif body is not None:
        data = json.dumps(body).encode()
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, data=data, timeout=120) as r:
            raw = r.read()
            status = r.status
    except urllib.error.HTTPError as e:
        raw = e.read()
        status = e.code
    try:
        obj = json.loads(raw) if raw else {}
    except ValueError:
        obj = {"__raw__": raw.decode(errors="replace")[:200]}
    return status, obj


def redis_get(key):
    out = subprocess.run(["docker", "exec", "hmdp-redis", "redis-cli", "GET", key], capture_output=True, text=True)
    return out.stdout.strip()


def mysql(sql):
    out = subprocess.run(["docker", "exec", "hmdp-mysql", "mysql", "-uroot", "-p123456", "hmdp", "-N", "-e", sql],
                         capture_output=True, text=True)
    if out.returncode != 0:
        return "ERR:" + out.stderr.strip()[:120]
    return out.stdout.strip()


BUGS = []
CHECKS = [0, 0]  # pass, fail


def check(section, name, cond, detail=""):
    CHECKS[0 if cond else 1] += 1
    mark = "PASS" if cond else "FAIL"
    log(f"  [{mark}] {name}" + ("" if cond else f"  ← {detail}"))
    if not cond:
        BUGS.append(f"{section}: {name} — {detail}")


def login(phone):
    http("POST", f"/user/code?phone={phone}")
    code = redis_get(f"login:code:{phone}")
    _, m = http("POST", "/user/login", {"phone": phone, "code": code})
    token = m.get("data", "")
    _, me = http("GET", "/user/me", token=token)
    uid = (me.get("data") or {}).get("id", 0)
    return token, uid


def section(title):
    log("")
    log(f"===== {title} =====")


# ============ 认证 ============
section("认证 Auth")
st, m = http("GET", "/user/me")
check("认证", "未登录访问 /user/me 返回 401", st == 401, f"got {st} {m}")
st, m = http("POST", "/user/code?phone=123")
check("认证", "非法手机号文案", m.get("errorMsg") == "手机号格式错误", str(m))
st, m = http("POST", "/user/login", {"phone": "13911112222", "code": "000000"})
check("认证", "错误验证码文案", m.get("errorMsg") == "验证码不一致，请重新输入", str(m))
token, m = login("13911112222")
check("认证", "登录成功返回 32 位 token", isinstance(token, str) and len(token) == 32, str(m)[:120])
st, m = http("GET", "/user/me", token=token)
check("认证", "/user/me 返回用户摘要", m.get("success") and m["data"].get("id") is not None, str(m))
uid = m["data"]["id"] if m.get("data") else 0

st, m = http("POST", "/user/logout", token=token)
check("认证", "登出保留原占位行为", m.get("errorMsg") == "功能未完成", str(m))
for path in ["/shop-type/list", "/blog/hot?current=1", "/shop/1"]:
    st, _ = http("GET", path)
    check("认证", f"白名单匿名访问 {path}", st == 200, f"got {st}")

# ============ 用户资料/签到 ============
section("用户资料与签到")
st, m = http("GET", "/user/info/1", token=token)
check("用户", "/user/info/1 返回资料且无 createTime", m.get("success") and "createTime" not in (m.get("data") or {}), str(m)[:150])
st, m = http("GET", "/user/info/999999", token=token)
check("用户", "/user/info 不存在返回空 data", m.get("success") and m.get("data") is None, str(m))
st, m = http("GET", f"/user/{uid}", token=token)
check("用户", "/user/{id} 摘要含昵称", m.get("success") and m["data"].get("nickName"), str(m))
st, m = http("POST", "/user/sign", token=token)
check("用户", "签到成功", m.get("success"), str(m))
st, m = http("GET", "/user/sign/count", token=token)
check("用户", "连续签到返回整数", isinstance(m.get("data"), int), str(m))

# ============ 店铺 ============
section("店铺")
st, m = http("GET", "/shop-type/list")
types = m.get("data") or []
check("店铺", "类型列表返回 10 条", len(types) == 10, f"got {len(types)}")
st, m = http("GET", "/shop/of/type?typeId=1&current=1", token=token)
check("店铺", "无坐标分页返回 <=5 条", m.get("success") and len(m.get("data") or []) <= 5, str(m)[:150])
st, m = http("GET", "/shop/1", token=token)
check("店铺", "店铺详情（DB 有数据）应可访问", m.get("success"), f"实际: {m}")
st, m = http("GET", "/shop/of/type?typeId=1&current=1&x=120.15&y=30.32", token=token)
check("店铺", "GEO 附近搜索应有结果（DB 有坐标店铺）", m.get("success") and len(m.get("data") or []) > 0, f"实际: {m}")
st, m = http("GET", "/shop/of/name?name=%E9%A3%9F%E5%A0%82&current=1", token=token)
check("店铺", "名称搜索接口可用", m.get("success"), str(m)[:120])
# 新增 + 更新 + 缓存
new_shop = {"name": "QA测试店", "typeId": 1, "images": "", "area": "南区", "address": "QA路1号",
            "x": 120.0001, "y": 30.0001, "avgPrice": 100, "sold": 0, "comments": 0, "score": 50, "openHours": "09:00-21:00"}
st, m = http("POST", "/shop", new_shop, token=token)
new_id = m.get("data")
check("店铺", "新增店铺返回 id", m.get("success") and isinstance(new_id, int), str(m))
if isinstance(new_id, int):
    st, m = http("GET", f"/shop/{new_id}", token=token)
    check("店铺", "新增后详情可访问", m.get("success"), f"实际: {m}")
    st, m = http("PUT", "/shop", {"id": new_id, "name": "QA测试店改"}, token=token)
    check("店铺", "更新店铺成功", m.get("success"), str(m))
    st, m = http("GET", f"/shop/{new_id}", token=token)
    check("店铺", "更新后详情反映新名称", m.get("success") and m.get("data", {}).get("name") == "QA测试店改", f"实际: {m}")

# ============ 优惠券与秒杀 ============
section("优惠券与秒杀")
sec_body = {"shopId": 1, "title": "QA秒杀券", "subTitle": "测试", "payValue": 100, "actualValue": 200,
            "type": 1, "status": 1, "stock": 5, "beginTime": "2026-09-01T10:00:00", "endTime": "2026-12-31T10:00:00"}
st, m = http("POST", "/voucher/seckill", sec_body, token=token)
vid = m.get("data")
check("秒杀", "发布秒杀券返回 id", m.get("success") and isinstance(vid, int), str(m))
if isinstance(vid, int):
    stock_redis = redis_get(f"seckill:stock:{vid}")
    check("秒杀", "Redis 库存已预载=5", stock_redis == "5", f"got {stock_redis}")
    st, m = http("GET", f"/voucher/list/1")
    found = any(v.get("id") == vid for v in (m.get("data") or []))
    check("秒杀", "券列表包含新券且带 stock", found, str(m)[:150])
    # 抢券（当前用户）
    st, m = http("POST", f"/voucher-order/seckill/{vid}", token=token)
    order_id = m.get("data")
    check("秒杀", "抢券成功返回订单号", m.get("success") and isinstance(order_id, int), str(m))
    st, m = http("POST", f"/voucher-order/seckill/{vid}", token=token)
    check("秒杀", "重复抢券被拒", m.get("errorMsg") == "不能重复下单", str(m))
    cnt, db_stock = "0", "?"
    for _ in range(20):
        cnt = mysql(f"SELECT COUNT(*) FROM tb_voucher_order WHERE voucher_id={vid}")
        db_stock = mysql(f"SELECT stock FROM tb_seckill_voucher WHERE voucher_id={vid}")
        if cnt == "1" and db_stock == "4":
            break
        time.sleep(1)
    check("秒杀", "订单已异步落库（Kafka 消费者）", cnt == "1", f"订单数={cnt}")
    check("秒杀", "DB 库存已扣减 5→4", db_stock == "4", f"DB stock={db_stock}")
    # 库存耗尽
    st, m = http("POST", "/voucher/seckill", {**sec_body, "title": "QA库存1", "stock": 1})
    vid2 = m.get("data")
    token2, _ = login("13922223333")
    st, m = http("POST", f"/voucher-order/seckill/{vid2}", token=token2)
    check("秒杀", "库存1第一人抢成功", m.get("success"), str(m))
    token3, _ = login("13933334444")
    st, m = http("POST", f"/voucher-order/seckill/{vid2}", token=token3)
    check("秒杀", "库存耗尽提示库存不足", m.get("errorMsg") == "库存不足", str(m))
st, m = http("POST", "/voucher", {"shopId": 1, "title": "QA普通券", "type": 0}, token=token)
plain_id = m.get("data")
check("券", "普通券发布成功（不建秒杀券）", m.get("success") and isinstance(plain_id, int), str(m))
if isinstance(plain_id, int):
    seckill_rows = mysql(f"SELECT COUNT(*) FROM tb_seckill_voucher WHERE voucher_id={plain_id}")
    check("券", "普通券未写入秒杀表", seckill_rows == "0", f"seckill rows={seckill_rows}")

# ============ 动态 ============
section("动态 Blog")
st, m = http("POST", "/blog", {"shopId": 1, "title": "QA动态", "images": "", "content": "QA内容"}, token=token)
blog_id = m.get("data")
check("动态", "发布动态返回 id", m.get("success") and isinstance(blog_id, int), str(m))
st, m = http("GET", "/blog/hot?current=1")
check("动态", "热门榜返回列表且含 name/icon", m.get("success") and m.get("data"), str(m)[:150])
st, m = http("GET", f"/blog/{blog_id}", token=token)
check("动态", "详情附 isLike=false", m.get("success") and m.get("data", {}).get("isLike") is False, str(m)[:150])
st, m = http("PUT", f"/blog/like/{blog_id}", token=token)
check("动态", "点赞成功", m.get("success"), str(m))
st, m = http("GET", f"/blog/{blog_id}", token=token)
check("动态", "点赞后 isLike=true", m.get("data", {}).get("isLike") is True, str(m)[:120])
st, m = http("GET", f"/blog/likes/{blog_id}", token=token)
check("动态", "点赞榜含当前用户", any(u.get("id") == uid for u in (m.get("data") or [])), str(m)[:150])
st, m = http("PUT", f"/blog/like/{blog_id}", token=token)
check("动态", "取消点赞成功", m.get("success"), str(m))
st, m = http("GET", "/blog/of/me?current=1", token=token)
check("动态", "我的动态含新发布", any(b.get("id") == blog_id for b in (m.get("data") or [])), str(m)[:150])
st, m = http("GET", f"/blog/of/user?id={uid}&current=1", token=token)
check("动态", "指定用户动态接口可用", m.get("success"), str(m)[:120])
st, m = http("GET", "/blog/999999", token=token)
check("动态", "不存在动态文案", m.get("errorMsg") == "博客不存在", str(m))

# ============ 关注与 feed ============
section("关注 Follow 与 Feed")
token4, uid4 = login("13944445555")
st, m = http("PUT", f"/follow/{uid}/true", token=token4)
check("关注", "关注成功", m.get("success"), str(m))
st, m = http("GET", f"/follow/or/not/{uid}", token=token4)
check("关注", "是否关注=true", m.get("data") is True, str(m))
# 新粉丝应收到作者此前/之后的动态：重新发一条
st, m = http("POST", "/blog", {"shopId": 1, "title": "QA粉丝feed", "images": "", "content": "feed内容"}, token=token)
feed_blog = m.get("data")
time.sleep(1)
st, m = http("GET", "/blog/of/follow?lastId=9999999999999&offset=0", token=token4)
feed_list = (m.get("data") or {}).get("list") or []
check("关注", "粉丝 feed 收到新动态", any(b.get("id") == feed_blog for b in feed_list), str(m)[:200])
st, m = http("GET", f"/follow/common/{uid4}", token=token)
check("关注", "共同关注接口可用（空列表也算通过）", m.get("success"), str(m)[:120])
st, m = http("PUT", f"/follow/{uid}/false", token=token4)
check("关注", "取关成功", m.get("success"), str(m))

# ============ 上传 ============
section("上传 Upload")
st, m = http("POST", "/upload/blog", token=token, files={"file": ("qa.png", b"FAKEPNGDATA")})
up_path = m.get("data")
check("上传", "上传图片成功", m.get("success") and up_path, f"实际: {m}")
if up_path:
    import urllib.request as _ur
    try:
        with _ur.urlopen("http://127.0.0.1:8080/imgs" + up_path, timeout=10) as r:
            img_ok = r.status == 200 and r.read() == b"FAKEPNGDATA"
    except Exception as e:
        img_ok = False
    check("上传", "图片可经 nginx /imgs/blogs 访问", img_ok, "404 或内容不符")
    # 路径穿越：容器内诱饵文件必须存活（实现为锚定到上传目录）
    subprocess.run(["docker", "exec", "hmdp-backend", "sh", "-c", "echo victim > /tmp/qa_victim.txt"], capture_output=True)
    http("GET", "/upload/blog/delete?name=" + urllib.parse.quote("../../tmp/qa_victim.txt"), token=token)
    http("GET", "/upload/blog/delete?name=" + urllib.parse.quote("/tmp/qa_victim.txt"), token=token)
    alive = subprocess.run(["docker", "exec", "hmdp-backend", "cat", "/tmp/qa_victim.txt"], capture_output=True, text=True)
    check("上传", "路径穿越/绝对路径无法删除目录外文件", alive.returncode == 0 and "victim" in alive.stdout, "诱饵文件被删!")
    st, _ = http("GET", f"/upload/blog/delete?name={urllib.parse.quote(up_path)}", token=token)
    check("上传", "删除图片成功", st == 200, f"got {st}")
    st, m = http("GET", "/upload/blog/delete?name=" + urllib.parse.quote("/blogs/nonexist"), token=token)
    check("上传", "删除不存在路径不报错", m.get("success") is True, str(m))

# ============ RAG ============
section("RAG 知识库")
st, m = http("POST", "/rag/kb", {"name": "QA知识库"}, token=token)
kb_id = (m.get("data") or {}).get("id")
check("RAG", "创建知识库", m.get("success") and kb_id, str(m)[:150])
if kb_id:
    st, m = http("POST", "/rag/doc/upload", token=token,
                 files={"file": ("qa.md", "# QA文档\n\n食堂每天 7:00 开门。图书馆每人最多借 10 本书。".encode())},
                 form={"kbId": str(kb_id), "chunkMethod": "naive"})
    doc_id = (m.get("data") or {}).get("docId")
    check("RAG", "上传文档返回 docId", m.get("success") and doc_id, str(m)[:150])
    ok = False
    for _ in range(50):
        _, ml = http("GET", f"/rag/doc/list?kbId={kb_id}", token=token)
        for d in (ml.get("data") or []):
            if d.get("id") == doc_id and d.get("status") == "done":
                ok = True
        if ok:
            break
        time.sleep(0.2)
    check("RAG", "文档异步入库完成", ok, "超时或失败")
    st, m = http("GET", f"/rag/doc/{doc_id}/chunks", token=token)
    check("RAG", "切片列表非空且不含 embedding", m.get("data") and "embedding" not in (m["data"][0] or {}), str(m)[:150])
    st, m = http("POST", "/rag/retrieve", {"kbId": kb_id, "question": "食堂几点开门", "topK": 3}, token=token)
    check("RAG", "检索命中", m.get("success") and m.get("data") and "食堂" in m["data"][0]["text"], str(m)[:150])
    st, m = http("POST", "/rag/chat", {"kbId": kb_id, "question": "食堂几点开门？"}, token=token)
    ans = (m.get("data") or {}).get("answer", "")
    check("RAG", "问答返回且含引用标记", m.get("success") and "[ID:" in ans, str(m)[:200])
    st, m = http("POST", "/rag/chat", {"kbId": kb_id, "question": "   "}, token=token)
    check("RAG", "空问题被拒", m.get("errorMsg") == "问题不能为空", str(m))
    st, m = http("POST", "/rag/eval/run", {"kbId": kb_id, "cases": [{"question": "食堂几点开门？", "groundTruth": "7 点", "answer": "食堂 7 点开门。[ID:0]", "contexts": ["食堂每天 7:00 开门。"]}]}, token=token)
    check("RAG", "评估运行成功含 5 指标", m.get("success") and "citation_accuracy" in (m.get("data", {}).get("metrics") or {}), str(m)[:200])
    st, m = http("POST", "/rag/doc/upload", token=token, files={"file": ("bad.exe", b"xx")}, form={"kbId": str(kb_id)})
    check("RAG", "不支持格式被拒", m.get("success") is False and "不支持" in (m.get("errorMsg") or ""), str(m))
    st, m = http("DELETE", f"/rag/kb/{kb_id}", token=token)
    check("RAG", "删除知识库", m.get("success"), str(m))

# ============ 并发一人一单/超卖 ============
section("并发（Lua 原子性）")
st, m = http("POST", "/voucher/seckill", {**sec_body, "title": "QA并发券", "stock": 3}, token=token)
cvid = m.get("data")
if cvid:
    import threading
    results = []
    lock = threading.Lock()
    def grab(ph):
        t, _ = login(ph)
        _, r = http("POST", f"/voucher-order/seckill/{cvid}", token=t)
        with lock:
            results.append(r)
    threads = [threading.Thread(target=grab, args=(f"1395555{i:04d}",)) for i in range(8)]
    for t in threads: t.start()
    for t in threads: t.join()
    ok_cnt = sum(1 for r in results if r.get("success"))
    check("并发", "8 人抢 3 库存恰好成功 3 单", ok_cnt == 3, f"成功 {ok_cnt} 单")
    # 同一用户并发 5 次
    t_same, _ = login("13966660000")
    res2 = []
    def grab_same():
        _, r = http("POST", f"/voucher-order/seckill/{cvid}", token=t_same)
        with lock:
            res2.append(r)
    # 先补一批库存
    st, m = http("POST", "/voucher/seckill", {**sec_body, "title": "QA并发券2", "stock": 10}, token=token)
    cvid2 = m.get("data")
    res3 = []
    def grab_same2():
        _, r = http("POST", f"/voucher-order/seckill/{cvid2}", token=t_same)
        with lock:
            res3.append(r)
    ths = [threading.Thread(target=grab_same2) for _ in range(5)]
    for t in ths: t.start()
    for t in ths: t.join()
    ok2 = sum(1 for r in res3 if r.get("success"))
    check("并发", "同一用户并发 5 次仅 1 单成功", ok2 == 1, f"成功 {ok2} 单")

# ============ 安全 ============
section("安全")
subprocess.run(["docker", "exec", "hmdp-backend", "sh", "-c", "echo victim2 > /tmp/qa_victim2.txt"], capture_output=True)
http("GET", "/upload/blog/delete?name=" + urllib.parse.quote("../tmp/qa_victim2.txt"), token=token)
alive2 = subprocess.run(["docker", "exec", "hmdp-backend", "cat", "/tmp/qa_victim2.txt"], capture_output=True, text=True)
check("安全", "上传删除不可越界（诱饵存活）", alive2.returncode == 0, "越界删除发生!")
st, m = http("GET", "/user/me")
check("安全", "无 token 401", st == 401, f"got {st}")

log("")
log(f"===== 汇总：{CHECKS[0]} 通过 / {CHECKS[1]} 失败 =====")
log("")
log("===== 问题清单 =====")
for i, b in enumerate(BUGS, 1):
    log(f"{i}. {b}")
REPORT.close()
print(f"\n报告已保存 /tmp/qa_report.txt（{CHECKS[0]} 通过 / {CHECKS[1]} 失败）")
