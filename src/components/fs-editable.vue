<template>
  <div class="fs-editable" :class="{ disabled, 'hover-show': hoverShow }">
    <div v-if="isEdit" class="input">
      <a-input
        ref="inputRef"
        v-model:value="valueRef"
        :validate-status="modelValue ? '' : 'error'"
        v-bind="input"
        @keyup.enter="save()"
        @blur="save()"
      >
        <template #suffix>
          <CheckOutlined style="color: rgba(0, 0, 0, 0.45)" @click="save()" />
        </template>
      </a-input>
    </div>
    <div v-else class="view" @click="edit">
      <span> {{ modelValue }}</span>
      <EditOutlined class="ml-5 edit-icon" />
    </div>
  </div>
</template>

<script>
import { watch, ref, nextTick } from "vue";

export default {
  name: "FsEditable",
  props: {
    modelValue: {
      type: String,
      default: ""
    },
    input: {
      type: Object
    },
    disabled: {
      type: Boolean,
      default: false
    },
    hoverShow: {
      type: Boolean,
      default: false
    }
  },
  emits: ["update:modelValue"],
  setup(props, ctx) {
    const inputRef = ref();
    const valueRef = ref(props.modelValue);
    watch(
      () => {
        return props.modelValue;
      },
      (value) => {
        valueRef.value = value;
      }
    );
    const isEdit = ref(false);
    async function edit() {
      if (props.disabled) {
        return;
      }
      isEdit.value = true;
      await nextTick();
      inputRef.value.focus();
    }
    function save() {
      isEdit.value = false;
      ctx.emit("update:modelValue", valueRef.value);
    }
    return {
      valueRef,
      isEdit,
      save,
      edit,
      inputRef
    };
  }
};
</script>

<style lang="less">
.fs-editable {
  line-height: 34px;

  &.disabled {
    .edit-icon {
      visibility: hidden !important;
    }
  }
  &.hover-show {
    .edit-icon {
      visibility: hidden;
    }
    &:hover {
      .edit-icon {
        visibility: visible;
      }
    }
  }
  .edit-icon {
    line-height: 34px;
  }
  .view {
    cursor: pointer;
    display: flex;
  }
}
</style>
