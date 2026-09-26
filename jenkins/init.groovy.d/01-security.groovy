// Jenkins 初始化：安全域、管理员账号、GitHub 部署密钥凭据、known_hosts
import jenkins.model.*
import hudson.security.*
import com.cloudbees.plugins.credentials.*
import com.cloudbees.plugins.credentials.domains.*
import com.cloudbees.jenkins.plugins.sshcredentials.impl.BasicSSHUserPrivateKey

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

// 2) GitHub 部署密钥凭据（id: github-deploy-key）
def keyFile = new File('/run/secrets/github_deploy_key')
if (keyFile.exists()) {
    def store = SystemCredentialsProvider.getInstance().getStore()
    def exists = store.getCredentials(Domain.global()).any { it.id == 'github-deploy-key' }
    if (!exists) {
        def cred = new BasicSSHUserPrivateKey(
            CredentialsScope.GLOBAL,
            'github-deploy-key',
            'git',
            new BasicSSHUserPrivateKey.DirectEntryPrivateKeySource(keyFile.text),
            null,
            'GitHub 部署密钥（campus-service-platform）')
        store.addCredentials(Domain.global(), cred)
        println('已创建 GitHub 部署密钥凭据 github-deploy-key')
    }
}

// 3) github.com known_hosts（首次克隆免交互）
try {
    def sshDir = new File('/root/.ssh')
    sshDir.mkdirs()
    def known = new File(sshDir, 'known_hosts')
    def scan = 'ssh-keyscan -H github.com'.execute()
    scan.waitFor()
    if (scan.exitValue() == 0) {
        known.text = scan.in.text
        println('github.com known_hosts 已写入')
    }
} catch (Exception e) {
    println("known_hosts 写入失败: ${e.message}")
}
