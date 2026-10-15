<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { http, errorText } from '../api'
import { useSession } from '../session'

const form = reactive({ username: '', password: '' })
const hint = ref(null)
const error = ref('')
const loading = ref(false)
const router = useRouter()
const session = useSession()

onMounted(async () => {
  try {
    const { data } = await http.post('/auth/bootstrap')
    if (data?.data?.username && data?.data?.password) {
      hint.value = data.data
      form.username = data.data.username
      form.password = data.data.password
    }
  } catch {
    hint.value = null
  }
})

async function submit() {
  error.value = ''
  loading.value = true
  try {
    const { data } = await http.post('/auth/login', form)
    session.apply(data.data)
    router.push('/')
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <div class="card login-card stack">
      <h1 class="page-title">EduGo</h1>
      <p class="lede">登录后继续课程、练习和复习。账号由管理员创建。</p>
      <p v-if="hint" class="lede">开发环境默认账号 {{ hint.username }} / {{ hint.password }}，未配置管理员密码时可用。</p>
      <el-input v-model="form.username" placeholder="用户名" @keyup.enter="submit" />
      <el-input v-model="form.password" type="password" show-password placeholder="密码" @keyup.enter="submit" />
      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
      <el-button type="primary" :loading="loading" @click="submit">登录</el-button>
      <router-link to="/verify">核验证书</router-link>
    </div>
  </div>
</template>
