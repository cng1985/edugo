<script setup>
import { useRouter } from 'vue-router'
import { useSession } from '../session'
import { post } from '../api'

const session = useSession()
const router = useRouter()

const links = [
  { to: '/', label: '首页' },
  { to: '/courses', label: '课程' },
  { to: '/study', label: '学习' },
  { to: '/questions', label: '题库', teach: true },
  { to: '/exams', label: '考试' },
  { to: '/review', label: '复习' },
  { to: '/plans', label: '计划' },
  { to: '/certificates', label: '证书' },
  { to: '/inbox', label: '通知' },
  { to: '/tutor', label: '辅导' },
  { to: '/reports', label: '报告' },
  { to: '/knowledge', label: '知识' },
  { to: '/users', label: '用户', admin: true },
]

async function logout() {
  try {
    await post('/auth/logout', {})
  } catch {
    /* 本地会话仍会清除 */
  }
  session.clear()
  router.push('/login')
}
</script>

<template>
  <div class="shell">
    <aside class="side">
      <div class="brand">
        <strong>EduGo</strong>
        <span>把今天的一小节学完</span>
      </div>
      <nav class="nav">
        <router-link v-for="link in links" :key="link.to" :to="link.to" v-show="(!link.teach || session.canTeach) && (!link.admin || session.isAdmin)">
          {{ link.label }}
        </router-link>
      </nav>
    </aside>
    <section class="main">
      <div class="topbar">
        <div class="who">{{ session.user?.displayName }} · {{ (session.roles || []).join(' / ') }}</div>
        <el-button @click="logout">退出</el-button>
      </div>
      <router-view />
    </section>
  </div>
</template>
