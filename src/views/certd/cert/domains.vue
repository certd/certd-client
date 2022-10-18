<template>
  <a-tooltip v-if="!expand" :title="value">
    <a-badge :count="badgeCount" color="green">
      <component :is="type" color="cyan" class="domain-item">{{ list[0] }}</component>
    </a-badge>
  </a-tooltip>
  <div v-else>
    <component :is="type" v-for="item of list" color="cyan" class="domain-item">{{ item }}</component>
  </div>
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
    },
    expand: {
      type: Boolean,
      default: false
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
