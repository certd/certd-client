<template>
  <div class="access-selector">
    <span v-if="target.name" class="mlr-5">{{ target.name }}</span>
    <span v-else class="mlr-5 gray">请选择</span>
    <a-button @click="chooseForm.open">选择</a-button>
    <a-form-item-rest>
      <a-modal v-model:visible="chooseForm.show" title="选择授权提供者" @ok="chooseForm.ok">
        <div class="d-container access-provider-manager">
          <a-button @click="editForm.open()"> 添加授权 </a-button>
          <a-list class="list" item-layout="horizontal" :data-source="accessList">
            <template #renderItem="{ item }">
              <a-list-item>
                <template #actions>
                  <a-button type="primary" @click="editForm.open(item.id)">
                    <template #icon>
                      <EditOutlined />
                    </template>
                  </a-button>
                  <a-button type="danger" @click="editForm.delete(item.id)">
                    <template #icon>
                      <DeleteOutlined />
                    </template>
                  </a-button>
                </template>

                <a-radio :checked="item.id === selectedId" @update:checked="selectedId = item.id">
                  {{ item.name }} ({{ item.type }})
                </a-radio>
              </a-list-item>
            </template>
          </a-list>
        </div>
      </a-modal>

      <a-modal v-model:visible="editForm.show" dialog-class="d-dialog" width="700px" title="编辑授权" @ok="editForm.ok">
        <a-alert v-if="providerDefine?.desc" :message="providerDefine.desc" type="success" />

        <a-form ref="formRef" class="domain-form" :model="editForm.data" label-width="150px">
          <template v-if="providerDefine">
            <a-form-item label="名称" name="name" :rules="editForm.rules.name">
              <a-input v-model:value="editForm.data.name" />
            </a-form-item>
            <a-form-item
              v-for="(item, key, index) in providerDefine.input"
              :key="index"
              v-bind="item.component || {}"
              :label="item.label || key"
              :name="key"
            >
              <component-render v-model:value="editForm.data[key]" v-bind="item.component || {}"></component-render>
              <template #extra>
                <div v-if="item.desc" class="helper">{{ item.desc }}</div>
              </template>
            </a-form-item>
          </template>
        </a-form>
      </a-modal>
    </a-form-item-rest>
  </div>
</template>

<script>
import { defineComponent, reactive, ref, watch } from "vue";
import * as api from "./api";

export default defineComponent({
  name: "AccessSelector",
  props: {
    modelValue: {
      type: String,
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
