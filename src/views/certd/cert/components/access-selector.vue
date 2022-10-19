<template>
  <div class="access-selector">
    <span v-if="target.name" class="mlr-5">{{ target.name }}</span>
    <span v-else class="mlr-5 gray">请选择</span>
    <a-button @click="chooseForm.open">选择</a-button>
    <a-form-item-rest>
      <a-modal v-model:visible="chooseForm.show" title="选择授权提供者" width="700px" @ok="chooseForm.ok">
        <div style="height: 400px; position: relative">
          <cert-access-modal v-model="selectedId" :type="type"></cert-access-modal>
        </div>
      </a-modal>
    </a-form-item-rest>
  </div>
</template>

<script>
import { defineComponent, reactive, ref, watch } from "vue";
import * as api from "./api";
import CertAccessModal from "./access/index.vue";

export default defineComponent({
  name: "AccessSelector",
  components: { CertAccessModal },
  props: {
    modelValue: {
      type: [Number, String],
      default: null
    },
    type: {
      type: String,
      default: "aliyun"
    }
  },
  emits: ["update:modelValue"],
  setup(props, ctx) {
    const target = ref({});
    const selectedId = ref();
    async function refreshTarget(value) {
      selectedId.value = value;
      target.value = await api.GetObj(value);
    }
    watch(
      () => {
        return props.modelValue;
      },
      async (value) => {
        if (value == null) {
          return;
        }
        await refreshTarget(value);
      },
      {
        immediate: true
      }
    );

    const accessList = ref([]);
    const providerDefine = ref({});
    async function refreshAccessList(value) {
      accessList.value = await api.GetList(value);
    }
    async function refreshProviderDefine(type) {
      providerDefine.value = await api.GetProviderDefine(type);
    }
    watch(
      () => {
        return props.type;
      },
      async (value) => {
        await refreshAccessList(value);
        await refreshProviderDefine(value);
      },
      {
        immediate: true
      }
    );

    const chooseForm = reactive({
      show: false,
      open() {
        chooseForm.show = true;
      },
      ok: () => {
        chooseForm.show = false;
        refreshTarget(selectedId.value);
        ctx.emit("update:modelValue", selectedId.value);
      }
    });

    // access crud
    const editForm = reactive({
      data: ref({ type: props.type }),
      show: false,
      rules: {
        name: [
          {
            type: "string",
            required: true,
            message: "请输入名称"
          }
        ]
      },
      open(id) {
        editForm.show = true;
        if (id > 0) {
          editForm.data.value = api.GetObj(id);
        }
      },
      async delete(id) {
        await api.DelObj(id);
        refreshAccessList();
      },
      async ok() {
        if (editForm.data.id > 0) {
          await api.UpdateObj(editForm.data);
        } else {
          await api.AddObj(editForm.data);
        }
        editForm.show = false;
        refreshAccessList();
      }
    });

    return {
      target,
      selectedId,
      accessList,
      providerDefine,
      chooseForm,
      editForm
    };
  }
});
</script>
<style lang="less">
.access-selector {
}
</style>
