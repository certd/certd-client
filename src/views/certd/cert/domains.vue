<template>
  <a-tooltip :title="value">
    <a-badge :count="badgeCount" color="green">
      <component :is="type" color="cyan" class="domain-item">{{ list[0] }}</component>
    </a-badge>
    <!--    <a-tag v-if="list.length > 1" a-tag>+{{ list.length - 1 }}</a-tag>-->
  </a-tooltip>
</template>

<script>
import { defineComponent, ref, onMounted, computed } from "vue";
export default defineComponent({
  name: "CertdDomains",
  props: {
    value: {
      type: String,
      default: ""
    },
    type: {
      type: String,
      default: "a-tag"
    }
  },
  setup(props) {
    const list = computed(() => {
      const domains = props.value || "";
      return domains.split(",");
    });

    const badgeCount = computed(() => {
      if (list.value.length <= 1) {
        return 0;
      }
      return "+" + (list.value.length - 1);
    });
    return {
      list,
      badgeCount
    };
  }
});
</script>
<style lang="less"></style>
