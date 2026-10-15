import { createRouter, createWebHistory } from 'vue-router'
import { useSession } from './session'
import Shell from './layouts/Shell.vue'
import Login from './views/Login.vue'
import Home from './views/Home.vue'
import Courses from './views/Courses.vue'
import Study from './views/Study.vue'
import Questions from './views/Questions.vue'
import Exams from './views/Exams.vue'
import Review from './views/Review.vue'
import Plans from './views/Plans.vue'
import Certificates from './views/Certificates.vue'
import Inbox from './views/Inbox.vue'
import Tutor from './views/Tutor.vue'
import Reports from './views/Reports.vue'
import Knowledge from './views/Knowledge.vue'
import Users from './views/Users.vue'
import Verify from './views/Verify.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login },
    { path: '/verify', component: Verify },
    {
      path: '/',
      component: Shell,
      children: [
        { path: '', component: Home },
        { path: 'courses', component: Courses },
        { path: 'study', component: Study },
        { path: 'questions', component: Questions },
        { path: 'exams', component: Exams },
        { path: 'review', component: Review },
        { path: 'plans', component: Plans },
        { path: 'certificates', component: Certificates },
        { path: 'inbox', component: Inbox },
        { path: 'tutor', component: Tutor },
        { path: 'reports', component: Reports },
        { path: 'knowledge', component: Knowledge },
        { path: 'users', component: Users },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const session = useSession()
  if (to.path === '/login' || to.path === '/verify') return true
  if (!session.accessToken) return '/login'
  return true
})
