<script setup>
import { onMounted, reactive, ref } from 'vue'
import { post, errorText } from '../api'

const rows = ref([])
const error = ref('')
const form = reactive({ username: '', password: '', displayName: '', role: 'student' })

async function load() {
  const res = await post('/user/list', { page: 1, size: 50 })
  rows.value = res.data || []
}

async function create() {
  error.value = ''
  try {
    await post('/user/create', form)
    form.username = ''
    form.password = ''
    form.displayName = ''
    await load()
  } catch (err) {
    error.value = errorText(err)
  }
}

async function toggle(user) {
  const path = user.status === 'active' ? '/user/disable' : '/user/enable'
  await post(path, { userId: user.id })
  await load()
}

onMounted(async () => {
  try { await load() } catch (err) { error.value = errorText(err) }
})
</script>

<template>
  <div class="stack">
    <h1 class="page-title">用户</h1>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <div class="card stack">
      <div class="row">
        <el-input class="grow" v-model="form.username" placeholder="用户名" />
        <el-input class="grow" v-model="form.displayName" placeholder="显示名" />
        <el-input class="grow" v-model="form.password" placeholder="至少 8 位密码" show-password type="password" />
        <el-select v-model="form.role" style="width: 120px">
          <el-option label="学生" value="student" />
          <el-option label="教师" value="teacher" />
          <el-option label="管理员" value="admin" />
        </el-select>
        <el-button type="primary" @click="create">创建</el-button>
      </div>
    </div>
    <div class="card row" v-for="user in rows" :key="user.id">
      <div class="grow">
        <strong>{{ user.displayName }}</strong>
        <div class="muted">{{ user.username }} · {{ (user.roles || []).join(',') }} · {{ user.status }}</div>
      </div>
      <el-button @click="toggle(user)">{{ user.status === 'active' ? '停用' : '启用' }}</el-button>
    </div>
  </div>
</template>
