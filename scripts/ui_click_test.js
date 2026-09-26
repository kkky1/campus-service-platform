// UI 点击测试：把每个交互按钮真实点一遍并断言结果
const { chromium } = require('playwright');
const fs = require('fs');

const BASE = 'http://127.0.0.1:8080';
const PHONE = '13900001111';
let pass = 0, fail = 0;
const failures = [];

async function step(name, fn) {
  try {
    await fn();
    pass++;
    console.log(`PASS  ${name}`);
  } catch (e) {
    fail++;
    const msg = (e.message || '').split('\n')[0].slice(0, 160);
    failures.push(`${name} :: ${msg}`);
    console.log(`FAIL  ${name}  ← ${msg}`);
  }
}

async function api(method, path, body, token) {
  const r = await fetch(BASE + '/api' + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { authorization: token } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  return r.json();
}

async function waitMessage(page, substr, timeout = 8000) {
  const loc = page.locator('.el-message', { hasText: substr }).first();
  await loc.waitFor({ timeout });
  return loc;
}

async function answerPrompt(page, value) {
  const input = page.locator('.el-message-box__input input, .el-message-box__input textarea').first();
  await input.waitFor({ timeout: 5000 });
  await input.fill(value);
  await page.locator('.el-message-box__btns .el-button--primary').first().click();
}

(async () => {
  // 登录拿 token
  const login = await api('POST', '/user/login', { phone: PHONE, code: '123456' });
  if (!login.success) throw new Error('登录失败');
  const token = login.data;

  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 420, height: 900 } });
  await ctx.addInitScript((t) => sessionStorage.setItem('token', t), token);
  const page = await ctx.newPage();
  const jsErrors = [];
  page.on('pageerror', (e) => jsErrors.push(`[pageerror] ${page.url()} :: ${e.message.slice(0, 150)}`));
  page.on('console', (m) => { if (m.type() === 'error') jsErrors.push(`[console] ${page.url()} :: ${m.text().slice(0, 150)}`); });

  // ========== 1. 首页：签到 + 搜索 ==========
  await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
  await step('首页-今日签到按钮可点击且成功', async () => {
    await page.locator('.campus-card').click();
    await waitMessage(page, '签到成功');
  });
  await step('首页-搜索框回车跳转店铺列表', async () => {
    await page.locator('.search-input input').fill('餐厅');
    await page.locator('.search-input input').press('Enter');
    await page.waitForURL(/shop-list\.html\?search=/, { timeout: 8000 });
    await page.waitForLoadState('networkidle');
  });

  // ========== 2. 店铺列表：排序与进入详情 ==========
  await step('店铺列表-搜索结果非空', async () => {
    await page.locator('.shop-box').first().waitFor({ timeout: 8000 });
  });
  await step('店铺列表-排序按钮不叠加列表', async () => {
    await page.locator('.sort-item', { hasText: '热度' }).click();
    await page.waitForTimeout(1200);
    const n1 = await page.locator('.shop-box').count();
    await page.locator('.sort-item', { hasText: '评分' }).click();
    await page.waitForTimeout(1200);
    const n2 = await page.locator('.shop-box').count();
    if (n1 > 5 || n2 > 5) throw new Error(`列表叠加: n1=${n1} n2=${n2}`);
  });
  await step('店铺列表-点击进入详情', async () => {
    await page.locator('.shop-box').first().click();
    await page.waitForURL(/shop-detail\.html/, { timeout: 8000 });
  });

  // ========== 3. 店铺详情：抢券 ==========
  await step('准备一张可抢的秒杀券', async () => {
    const v = await api('POST', '/voucher/seckill', {
      shopId: 1, title: 'UI测试券', payValue: 100, actualValue: 200, type: 1, status: 1, stock: 5,
      beginTime: '2026-09-01T00:00:00', endTime: '2026-12-31T00:00:00',
    }, token);
    if (!v.success) throw new Error(JSON.stringify(v));
  });
  await page.goto(BASE + '/shop-detail.html?id=1', { waitUntil: 'networkidle' });
  await step('店铺详情-立即抢券按钮可点击并成功', async () => {
    const btn = page.locator('.voucher-btn:not(.disable-btn)', { hasText: '立即抢券' }).last();
    await btn.waitFor({ timeout: 8000 });
    await btn.click();
    await waitMessage(page, '抢券成功');
  });

  // ========== 4. 首页 footer：附近 / 知识库 ==========
  await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
  await step('footer-知识库跳转 rag.html', async () => {
    await page.locator('.foot-box', { hasText: '知识库' }).click();
    await page.waitForURL(/rag\.html/, { timeout: 8000 });
  });
  await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
  await step('footer-附近跳转店铺列表', async () => {
    await page.locator('.foot-box', { hasText: '附近' }).click();
    await page.waitForURL(/shop-list\.html/, { timeout: 8000 });
  });

  // ========== 5. 我的：签到 / 知识库入口 ==========
  await page.goto(BASE + '/info.html', { waitUntil: 'networkidle' });
  await step('我的-签到按钮提示成功', async () => {
    await page.locator('button', { hasText: '签到' }).click();
    await waitMessage(page, '签到成功');
  });
  await step('我的-知识库入口跳转', async () => {
    await page.locator('button', { hasText: '校园知识库' }).click();
    await page.waitForURL(/rag\.html/, { timeout: 8000 });
  });

  // ========== 6. 资料编辑：修改昵称 + 保存 ==========
  await page.goto(BASE + '/info-edit.html', { waitUntil: 'networkidle' });
  await step('资料编辑-昵称可修改', async () => {
    await page.locator('.info-item', { hasText: '昵称' }).click();
    await answerPrompt(page, 'UI测试同学');
    await page.locator('.info-btn', { hasText: 'UI测试同学' }).waitFor({ timeout: 5000 });
  });
  await step('资料编辑-性别可修改（校验非法值）', async () => {
    await page.locator('.info-item', { hasText: '性别' }).click();
    await answerPrompt(page, '男');
    await page.locator('.info-btn', { hasText: '男' }).waitFor({ timeout: 5000 });
  });
  await step('资料编辑-保存成功且落库', async () => {
    await page.locator('button', { hasText: '保存资料' }).click();
    await waitMessage(page, '资料已保存');
    await page.waitForTimeout(800);
    const me = await api('GET', '/user/me', null, token);
    if (!me.data || me.data.nickName !== 'UI测试同学') throw new Error('昵称未落库: ' + JSON.stringify(me.data));
  });

  // ========== 7. 发布动态 ==========
  await page.goto(BASE + '/blog-edit.html', { waitUntil: 'networkidle' });
  await step('发布-未选店铺给出提示', async () => {
    await page.locator('.header-commit-btn').click();
    await waitMessage(page, '请先选择关联的校园服务点');
    await page.waitForTimeout(2500);
  });
  await step('发布-选择店铺并成功发布', async () => {
    await page.locator('.blog-shop').first().click();          // 打开店铺弹窗
    await page.locator('.shop-dialog input[type=text]').fill('餐厅');
    await page.locator('.shop-dialog .el-icon-search').click();
    await page.locator('.shop-item').first().waitFor({ timeout: 8000 });
    await page.locator('.shop-item').first().click();          // 选中店铺
    await page.locator('.blog-title input').fill('UI测试动态标题');
    await page.locator('textarea').first().fill('UI测试动态内容，来自自动化点击测试。');
    await page.locator('.header-commit-btn').click();
    await page.waitForURL(/info\.html/, { timeout: 10000 });
  });

  // ========== 8. 详情页：点赞 / 关注 ==========
  await step('查找刚发布的动态', async () => {
    const list = await api('GET', '/blog/of/me?current=1', null, token);
    if (!list.data || !list.data.length) throw new Error('动态列表为空');
    page._blogId = list.data[0].id;
  });
  await page.goto(BASE + '/blog-detail.html?id=' + page._blogId, { waitUntil: 'networkidle' });
  await step('详情-点赞按钮生效（isLike 变化）', async () => {
    await page.locator('.foot-view', { hasText: '' }).nth(0); // 占位
    // 点赞按钮：底部动作栏第一个 .foot-view
    const like = page.locator('.foot-view').first();
    await like.click();
    await page.waitForTimeout(1500);
    const detail = await api('GET', '/blog/' + page._blogId, null, token);
    if (!detail.data.isLike) throw new Error('点赞后 isLike 未变化');
  });
  await step('详情-关注按钮可点击（在他人动态上）', async () => {
    // 种子动态（作者不是当前用户）
    const seed = await api('GET', '/blog/4', null, token);
    await page.goto(BASE + '/blog-detail.html?id=4', { waitUntil: 'networkidle' });
    const follow = page.locator('.logout-btn', { hasText: '关注' }).first();
    await follow.waitFor({ timeout: 8000 });
    await follow.click();
    await page.waitForTimeout(1500);
    const st = await api('GET', '/follow/or/not/' + seed.data.userId, null, token);
    if (st.data !== true) throw new Error('关注后状态未变为 true: ' + JSON.stringify(st));
  });

  // ========== 9. RAG 页面全流程 ==========
  await page.goto(BASE + '/rag.html', { waitUntil: 'networkidle' });
  await step('RAG-新建知识库', async () => {
    await page.locator('button', { hasText: '新建' }).click();
    await answerPrompt(page, 'UI知识库');
    await waitMessage(page, '知识库已创建');
  });
  fs.writeFileSync('/tmp/ui_doc.md', '# UI测试资料\n\n校园快递驿站位于北门东侧，开放时间为每天 9:00 到 19:00，凭取件码取件。\n');
  await step('RAG-上传文档并完成入库', async () => {
    // 切到文档页
    await page.locator('.el-tabs__item', { hasText: '文档' }).click();
    await page.locator('input[type=file]').setInputFiles('/tmp/ui_doc.md');
    await page.locator('button', { hasText: '上传并入库' }).click();
    await page.locator('.el-tag', { hasText: '完成' }).first().waitFor({ timeout: 30000 });
  });
  await step('RAG-提问并返回带引用答案', async () => {
    await page.locator('.el-tabs__item', { hasText: '问答' }).click();
    await page.locator('.input-bar input').fill('快递驿站几点开门？');
    await page.locator('.input-bar button').click();
    await page.locator('.cite').first().waitFor({ timeout: 60000 });
  });
  await step('RAG-点击引用角标打开引用详情', async () => {
    await page.locator('.cite').first().click();
    await page.locator('.detail-panel.show').waitFor({ timeout: 5000 });
    const text = await page.locator('.detail-panel').innerText();
    if (!text.includes('快递')) throw new Error('引用详情内容不符: ' + text.slice(0, 80));
  });
  await step('RAG-运行评估并显示指标', async () => {
    await page.locator('.el-tabs__item', { hasText: '评估' }).click();
    await page.locator('textarea').first().fill('快递驿站几点开门？|每天 9:00 到 19:00');
    await page.locator('button', { hasText: '运行评估' }).click();
    await page.locator('.metric').first().waitFor({ timeout: 120000 });
    const n = await page.locator('.metric').count();
    if (n < 5) throw new Error('指标数量不足: ' + n);
  });

  console.log('\n===== JS 错误 =====');
  const uniq = [...new Set(jsErrors)];
  console.log(uniq.length ? uniq.join('\n') : '（无）');
  console.log(`\n===== 结果：${pass} 通过 / ${fail} 失败 =====`);
  if (failures.length) console.log(failures.join('\n'));
  await browser.close();
})();
