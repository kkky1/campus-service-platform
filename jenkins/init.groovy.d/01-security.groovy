// Jenkins 初始化：安全域、管理员账号、GitHub SSH（git sshCommand + known_hosts）
import jenkins.model.*
import hudson.security.*

def env = System.getenv()
def jenkins = Jenkins.get()

// 1) 管理员账号 + 权限（未配置安全域时执行一次）
if (!(jenkins.getSecurityRealm() instanceof HudsonPrivateSecurityRealm)) {
    def user = env.JENKINS_ADMIN_USER ?: 'admin'
    def pass = env.JENKINS_ADMIN_PASSWORD ?: 'changeme'
    def realm = new HudsonPrivateSecurityRealm(false)
    realm.createAccount(user, pass)
    jenkins.setSecurityRealm(realm)
    def strategy = new FullControlOnceLoggedInAuthorizationStrategy()
    strategy.setAllowAnonymousRead(false)
    jenkins.setAuthorizationStrategy(strategy)
    jenkins.save()
    println("Jenkins 安全域已初始化，管理员: ${user}")
}

// 2) GitHub SSH：stash 部署密钥 + 全局 git sshCommand + known_hosts
try {
    def home = new File('/root')
    def sshDir = new File(home, '.ssh')
    sshDir.mkdirs()
    def keySrc = new File('/run/secrets/github_deploy_key')
    def keyDst = new File(sshDir, 'id_ed25519')
    if (keySrc.exists()) {
        keyDst.text = keySrc.text
        keyDst.setReadable(false, false)
        keyDst.setReadable(true, true)   // 0600
        keyDst.setWritable(false, false)
        keyDst.setWritable(true, true)
    }
    def scan = ['bash', '-c', 'ssh-keyscan -H github.com 2>/dev/null'].execute()
    scan.waitFor()
    if (scan.exitValue() == 0) {
        def known = new File(sshDir, 'known_hosts')
        def old = known.exists() ? known.getText('UTF-8') : ''
        known.text = scan.in.text + old
        println('github.com known_hosts 已写入')
    }
    // 全局 git 使用部署密钥（checkout scm 走 CLI git）
    def gitconfig = new File(home, '.gitconfig')
    gitconfig.text = """[user]
\tname = jenkins
\temail = jenkins@localhost
[core]
\tsshCommand = ssh -i /root/.ssh/id_ed25519 -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new
[init]
\tdefaultBranch = main
"""
    println('git sshCommand 已配置（部署密钥）')
} catch (Exception e) {
    println("GitHub SSH 初始化失败: ${e.message}")
}
