Vue.component("footBar", {
  template: `
    <div class="foot">
    <div class="foot-box" :class="{active: activeBtn === 1}" @click="toPage(1)">
      <div class="foot-view"><i class="el-icon-s-home"></i></div>
      <div class="foot-text">校园</div>
    </div>
    <div class="foot-box" :class="{active: activeBtn === 2}" @click="toPage(2)">
      <div class="foot-view"><i class="el-icon-map-location"></i></div>
      <div class="foot-text">附近</div>
    </div>
    <div class="foot-box" @click="toPage(0)">
      <img class="add-btn" src="/imgs/add.png" alt="">
    </div>
    <div class="foot-box" :class="{active: activeBtn === 5}" @click="toPage(5)">
      <div class="foot-view"><i class="el-icon-reading"></i></div>
      <div class="foot-text">知识库</div>
    </div>
    <div class="foot-box" :class="{active: activeBtn === 4}" @click="toPage(4)">
      <div class="foot-view"><i class="el-icon-user"></i></div>
      <div class="foot-text">我的</div>
    </div>
  </div>
  `,
  data() {
    return {
    }
  },
  props: ['activeBtn'],
  methods: {
    toPage(i) {
      if (i === 0) {
        location.href = campusTheme.page("/blog-edit.html")
      } else if (i === 4) {
        location.href = campusTheme.page("/info.html")
      } else if (i === 1) {
        location.href = campusTheme.page("/index.html")
      } else if (i === 2) {
        // 附近：进入店铺列表页
        location.href = campusTheme.page("/shop-list.html?type=1&name=美食")
      } else if (i === 5) {
        // 知识库：RAG 问答页
        location.href = campusTheme.page("/rag.html")
      }
    }
  }
})
