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
@import './styles/ui-polish.css';

view,
text,
image,
button,
input,
textarea {
  box-sizing: border-box;
}
</style>
