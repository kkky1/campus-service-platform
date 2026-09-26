// Jenkins 初始化：创建流水线任务（SCM 轮询触发，等价原 systemd 定时拉取）
import jenkins.model.*

def jenkins = Jenkins.get()
def name = 'campus-service-platform-cd'

if (jenkins.getItem(name) == null) {
    def xml = '''<?xml version='1.1' encoding='UTF-8'?>
<flow-definition plugin="workflow-job">
  <actions/>
  <description>校园服务平台自动化部署：拉取 main → 构建测试 → 编译 → docker compose 部署 → 健康检查</description>
  <keepDependencies>false</keepDependencies>
  <properties/>
  <definition class="org.jenkinsci.plugins.workflow.cps.CpsScmFlowDefinition" plugin="workflow-cps">
    <scm class="hudson.plugins.git.GitSCM" plugin="git">
      <configVersion>2</configVersion>
      <userRemoteConfigs>
        <hudson.plugins.git.UserRemoteConfig>
          <url>git@github.com:kkky1/campus-service-platform.git</url>
          <credentialsId>github-deploy-key</credentialsId>
        </hudson.plugins.git.UserRemoteConfig>
      </userRemoteConfigs>
      <branches>
        <hudson.plugins.git.BranchSpec>
          <name>*/main</name>
        </hudson.plugins.git.BranchSpec>
      </branches>
      <doGenerateSubmoduleConfigurations>false</doGenerateSubmoduleConfigurations>
      <submoduleCfg class="empty-list"/>
      <extensions>
        <hudson.plugins.git.extensions.impl.CloneOption>
          <shallow>true</shallow>
          <depth>1</depth>
          <noTags>true</noTags>
          <honorRefspec>false</honorRefspec>
        </hudson.plugins.git.extensions.impl.CloneOption>
      </extensions>
    </hudson.plugins.git.GitSCM>
    <scriptPath>Jenkinsfile</scriptPath>
    <lightweight>true</lightweight>
  </definition>
  <triggers>
    <hudson.triggers.SCMTrigger>
      <spec>H/2 * * * *</spec>
      <ignorePostCommitHooks>false</ignorePostCommitHooks>
    </hudson.triggers.SCMTrigger>
  </triggers>
  <disabled>false</disabled>
</flow-definition>'''
    jenkins.createProjectFromXML(name, new ByteArrayInputStream(xml.getBytes('UTF-8')))
    println("流水线任务已创建: ${name}（每 2 分钟轮询 main）")
} else {
    println("流水线任务已存在: ${name}")
}
