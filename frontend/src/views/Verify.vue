<script setup>
import { ref } from 'vue'
import axios from 'axios'
import { errorText } from '../api'

const no = ref('')
const result = ref(null)
const error = ref('')

async function verify() {
  error.value = ''
  result.value = null
  try {
    const { data } = await axios.post('/api/v1/certificate/verify', { certificateNo: no.value })
    result.value = data.data
  } catch (err) {
    error.value = errorText(err)
  }
}
</script>

<template>
  <div class="login-wrap">
    <div class="card login-card stack">
      <h1 class="page-title">核验证书</h1>
      <p class="lede">输入证书编号。这个页面不需要登录。</p>
      <el-input v-model="no" placeholder="证书编号" />
      <el-button type="primary" @click="verify">核验</el-button>
      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
      <div v-if="result">状态：{{ result.status }} <span v-if="result.title">· {{ result.title }}</span></div>
      <router-link to="/login">返回登录</router-link>
    </div>
  </div>
</template>
