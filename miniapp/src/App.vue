<script>
import { initAppPush } from './utils/push'

export default {
  onLaunch() {
    // #ifdef APP-PLUS
    initAppPush()
    // #endif
  },
}
</script>

<style>
@import './styles/apple-mobile.css';
</style>
