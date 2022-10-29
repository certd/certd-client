<template>
  <span class="pi-status-show">
    <template v-if="type === 'icon'">
      <fs-icon v-bind="statusRef" />
    </template>
    <template v-if="type === 'tag'">
      <a-tag :color="statusRef.color">{{ statusRef.label }}</a-tag>
    </template>
  </span>
</template>

<script lang="ts">
import { defineComponent, ref, provide, Ref, watch } from "vue";
import { statusUtil } from "/@/views/certd/pipeline/pipeline/utils/util.status";
export default defineComponent({
  name: "PiStatusShow",
  props: {
    status: {
      type: String,
      default: ""
    },
    type: {
      type: String,
      default: "icon"
    }
  },
  setup(props, ctx) {
    const statusRef: Ref = ref({});
    statusRef.value = statusUtil.get(props.status);
    return {
      statusRef
    };
  }
});
</script>
<style lang="less">
.pi-status-show {
}
</style>
