// Jenkins 初始化：安装流水线所需插件（首次启动安装并自动重启）
import jenkins.model.*

def pm = Jenkins.get().pluginManager
def needed = ['workflow-aggregator', 'git', 'ssh-credentials', 'credentials']
def missing = needed.findAll { pm.getPlugin(it) == null }

if (missing) {
    println("需要安装插件: ${missing.join(', ')}")
    try {
        // 参数：插件列表、安装后重启、动态加载
        pm.install(missing, true, false)
        println('插件安装已提交，Jenkins 将自动重启')
    } catch (Exception e) {
        println("插件安装失败: ${e.message}")
    }
} else {
    println('所需插件已就绪')
}
