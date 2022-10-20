<template>
  <div class="value-show">
    <a-tag>{{ target[labelKey] }}</a-tag>
  </div>
</template>

<script>
import { defineComponent, reactive, ref, watch } from "vue";

export default defineComponent({
  name: "LabelShow",
  props: {
    modelValue: {
      type: [Number, String],
      default: null
    },
    labelKey: {
      type: String,
      default: "label"
    },
    getInfo: {
      type: Function
    }
  },
  setup(props, ctx) {
    const target = ref({});
    async function refreshTarget(value) {
      target.value = {};
      if (value) {
        target.value = await props.getInfo(value);
      }
    }
    watch(
      () => {
        return props.modelValue;
      },
      async (value) => {
        await refreshTarget(value);
      },
      {
        immediate: true
      }
    );

    return {
      target
    };
  }
});
</script>
<style lang="less">
.access-selector {
}
</style>
