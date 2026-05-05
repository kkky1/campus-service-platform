// let commonURL = "http://192.168.50.115:8081";
let commonURL = "/api";
// 设置后台服务地址
axios.defaults.baseURL = commonURL;
axios.defaults.timeout = 10000;
// request拦截器，将用户token放入头中
let token = sessionStorage.getItem("token");
const campusTheme = {
  assetVersion: "campus20260505c",
  campusName: "明德大学",
  serviceTypeMap: {
    "美食": "校园餐饮",
    "KTV": "社团娱乐",
    "丽人·美发": "生活服务",
    "健身运动": "体育健身",
    "按摩·足疗": "健康放松",
    "美容SPA": "形象护理",
    "亲子游乐": "亲友接待",
    "酒吧": "校园夜话",
    "轰趴馆": "活动场地",
    "美睫·美甲": "美学社团"
  },
  shopNameMap: {
    1: "一食堂茶餐窗口",
    2: "北苑同学火锅社",
    3: "学生活动中心简餐",
    4: "创客咖啡学习空间",
    5: "校园火锅福利点",
    6: "研究生公寓共享餐厅",
    7: "体育馆轻食补给站",
    8: "东区寿司便当铺",
    9: "后街暖锅小聚点",
    10: "学生活动中心K歌房",
    11: "音乐社排练空间",
    12: "毕业季派对活动室",
    13: "北区社团轰趴馆",
    14: "万象艺文K歌厅"
  },
  areaMap: {
    "大关": "南区生活街",
    "拱宸桥/上塘": "北区生活街",
    "运河上街": "学生活动中心",
    "北部新城": "体育馆周边",
    "水晶城": "综合服务楼",
    "远洋乐堤港": "创客街区",
    "D32天阳购物中心": "社团活动区"
  },
  addressMap: {
    1: "南区食堂一层 A103",
    2: "北苑生活街 2 号楼",
    3: "学生活动中心五层",
    4: "创客街区 B115",
    5: "综合服务楼六层",
    6: "研究生公寓共享厨房旁",
    7: "体育馆东门补给区",
    8: "东区生活街 B1",
    9: "后街小聚广场 5F",
    10: "学生活动中心四层",
    11: "综合服务楼六层音乐区",
    12: "创客街区四层",
    13: "社团活动区 5 层",
    14: "体育馆西侧艺文空间"
  },
  blogPresetMap: {
    4: {
      title: "迎新集市路线：社团摊位、二手教材和夜市都在这里",
      content: "开学第一周的校园集市很热闹，学生活动中心门口一路排到北苑生活街。<br/><br/>二手教材、社团招新、校园福利券都可以现场领，晚一点还会有音乐社路演。建议大家先收藏摊位地图，再去抢热门活动名额。"
    },
    5: {
      title: "食堂二楼隐藏窗口测评：预算 30 元也能吃得很稳",
      content: "今天和室友试了食堂二楼新开的轻食窗口，人均 30 元以内，排队速度也很友好。<br/><br/>适合下课后快速补给，学生认证后还能叠加平台福利券。"
    },
    6: {
      title: "周末操场骑行社体验课：新手也能直接报名",
      content: "体育馆旁边的骑行社这周末有体验课，平台上可以直接报名并查看剩余名额。<br/><br/>活动从基础骑行姿势开始，适合想找运动搭子的同学。"
    },
    7: {
      title: "周末操场骑行社体验课：新手也能直接报名",
      content: "体育馆旁边的骑行社这周末有体验课，平台上可以直接报名并查看剩余名额。<br/><br/>活动从基础骑行姿势开始，适合想找运动搭子的同学。"
    }
  },
  decorateType(type) {
    if (!type) return type;
    type.displayName = this.serviceTypeMap[type.name] || type.name;
    return type;
  },
  decorateTypes(types) {
    const seen = {};
    return (types || [])
      .filter(t => {
        if (!t || seen[t.id]) return false;
        seen[t.id] = true;
        return true;
      })
      .map(t => this.decorateType(t));
  },
  decorateShop(shop) {
    if (!shop) return shop;
    shop.displayName = this.shopNameMap[shop.id] || this.rewriteCampusText(shop.name);
    shop.displayArea = this.areaMap[shop.area] || this.rewriteCampusText(shop.area);
    shop.displayAddress = this.addressMap[shop.id] || this.rewriteCampusText(shop.address);
    shop.displayOpenHours = shop.openHours || "按服务点公示时间";
    return shop;
  },
  decorateShops(shops) {
    return (shops || []).map(s => this.decorateShop(s));
  },
  decorateVoucher(voucher) {
    if (!voucher) return voucher;
    voucher.displayTitle = this.rewriteCampusText(voucher.title || "校园福利券").replace("代金券", "校园福利券");
    voucher.displaySubTitle = this.rewriteCampusText(voucher.subTitle || "学生认证后可参与");
    return voucher;
  },
  decorateBlog(blog) {
    if (!blog) return blog;
    const preset = this.blogPresetMap[blog.id];
    blog.displayTitle = preset ? preset.title : this.rewriteCampusText(blog.title);
    blog.displayContent = preset ? preset.content : this.rewriteCampusText(blog.content);
    blog.displayName = blog.name || "校园同学";
    return blog;
  },
  rewriteCampusText(text) {
    return (text || "")
      .replace(/黑马点评/g, "校园服务平台")
      .replace(/商户/g, "校园服务点")
      .replace(/商家/g, "校园服务点")
      .replace(/店铺/g, "服务点")
      .replace(/探店/g, "校园分享")
      .replace(/达人/g, "同学")
      .replace(/网友/g, "同学")
      .replace(/优惠券/g, "校园福利券")
      .replace(/代金券/g, "校园福利券")
      .replace(/抢购/g, "抢券")
      .replace(/杭州/g, "明德大学");
  },
  page(path) {
    const separator = path.indexOf("?") > -1 ? "&" : "?";
    return path + separator + "_v=" + this.assetVersion;
  }
};
axios.interceptors.request.use(
  config => {
    if(token) config.headers['authorization'] = token
    return config
  },
  error => {
    console.log(error)
    return Promise.reject(error)
  }
)
axios.interceptors.response.use(function (response) {
  // 判断执行结果
  if (!response.data.success) {
    return Promise.reject(response.data.errorMsg)
  }
  return response.data;
}, function (error) {
  // 一般是服务端异常或者网络异常
  console.log(error)
  if(error.response.status == 401){
    // 未登录，跳转
    setTimeout(() => {
      location.href = "/login.html"
    }, 200);
    return Promise.reject("请先登录");
  }
  return Promise.reject("服务器异常");
});
axios.defaults.paramsSerializer = function(params) {
  let p = "";
  Object.keys(params).forEach(k => {
    if(params[k]){
      p = p + "&" + k + "=" + params[k]
    }
  })
  return p;
}
const util = {
  commonURL,
  getUrlParam(name) {
    let reg = new RegExp("(^|&)" + name + "=([^&]*)(&|$)", "i");
    let r = window.location.search.substr(1).match(reg);
    if (r != null) {
      return decodeURI(r[2]);
    }
    return "";
  },
  formatPrice(val) {
    if (typeof val === 'string') {
      if (isNaN(val)) {
        return null;
      }
      // 价格转为整数
      const index = val.lastIndexOf(".");
      let p = "";
      if (index < 0) {
        // 无小数
        p = val + "00";
      } else if (index === p.length - 2) {
        // 1位小数
        p = val.replace("\.", "") + "0";
      } else {
        // 2位小数
        p = val.replace("\.", "")
      }
      return parseInt(p);
    } else if (typeof val === 'number') {
      if (!val) {
        return null;
      }
      const s = val + '';
      if (s.length === 0) {
        return "0.00";
      }
      if (s.length === 1) {
        return "0.0" + val;
      }
      if (s.length === 2) {
        return "0." + val;
      }
      const i = s.indexOf(".");
      if (i < 0) {
        return s.substring(0, s.length - 2) + "." + s.substring(s.length - 2)
      }
      const num = s.substring(0, i) + s.substring(i + 1);
      if (i === 1) {
        // 1位整数
        return "0.0" + num;
      }
      if (i === 2) {
        return "0." + num;
      }
      if (i > 2) {
        return num.substring(0, i - 2) + "." + num.substring(i - 2)
      }
    }
  }
}
