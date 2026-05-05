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
    <div class="foot-box" :class="{active: activeBtn === 3}" @click="toPage(3)">
      <div class="foot-view"><i class="el-icon-chat-dot-round"></i></div>
      <div class="foot-text">互动</div>
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
      } else if (i === 1){
        location.href = campusTheme.page("/")
      }
    }
  }
})
