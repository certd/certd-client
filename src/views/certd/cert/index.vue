<template>
  <fs-page class="page-cert">
    <template #header>
      <div class="title">证书管理</div>
    </template>
    <fs-crud ref="crudRef" v-bind="crudBinding">
      <div class="cert-list">
        <a-list
          :grid="{ gutter: 16, xs: 1, sm: 2, md: 3, lg: 3, xl: 4, xxl: 4, xxxl: 6 }"
          :data-source="crudBinding.data"
          :loading="crudBinding.table.loading"
        >
          <template #renderItem="{ index, item }">
            <a-list-item>
              <a-card hoverable>
                <template #title>
                  <fs-icon icon="ion:ribbon-outline" style="margin-right: 10px"></fs-icon>
                  证书 {{ item.id }}
                </template>
                <template #extra>
                  <a-tooltip title="上次运行：2022-01-01 11:11:11">
                    <a-tag color="green">成功</a-tag>
                  </a-tooltip>
                </template>
                <div>
                  <div @click="goDetail(item)">证书域名：<certd-domains :value="item.domains"></certd-domains></div>
                </div>
                <template #actions>
                  <a-tooltip title="执行">
                    <fs-icon color="blue" icon="ion:play" />
                  </a-tooltip>
                  <a-tooltip title="详情">
                    <fs-icon icon="ion:eye" @click="goDetail(item)" />
                  </a-tooltip>
                  <a-tooltip title="编辑">
                    <fs-icon icon="ion:edit" @click="openEditDialog(item, index)" />
                  </a-tooltip>
                  <a-tooltip title="删除">
                    <fs-icon class="red" icon="ion:trash" @click="deleteCert(item, index)" />
                  </a-tooltip>
                </template>
              </a-card>
            </a-list-item>
          </template>
        </a-list>
      </div>
    </fs-crud>
  </fs-page>
</template>

<script>
import { defineComponent, ref, onMounted } from "vue";
import { useCrud } from "@fast-crud/fast-crud";
import createCrudOptions from "./crud";
import { useExpose } from "@fast-crud/fast-crud";
import { message, Modal, notification } from "ant-design-vue";
import CertdDomains from "./domains.vue";
import { useRouter } from "vue-router";
import * as api from "./api";
export default defineComponent({
  name: "CertdCert",
  components: { CertdDomains },
  setup() {
    // crud组件的ref
    const crudRef = ref();
    // crud 配置的ref
    const crudBinding = ref();
    // 暴露的方法
    const { expose } = useExpose({ crudRef, crudBinding });
    // 你的crud配置
    const { crudOptions } = createCrudOptions({ expose });
    // 初始化crud配置
    // eslint-disable-next-line @typescript-eslint/no-unused-vars,no-unused-vars
    const { resetCrudOptions } = useCrud({ expose, crudOptions });
    // 你可以调用此方法，重新初始化crud配置
    // resetCrudOptions(options)

    // 页面打开后获取列表数据
    onMounted(() => {
      expose.doRefresh();
    });

    const router = useRouter();
    function goDetail(item) {
      router.push({ path: "/certd/cert/detail", query: { id: item.id } });
    }

    function deleteCert(item, index) {
      expose.doRemove({ index, row: item });
    }
    function openEditDialog(item, index) {
      expose.openEdit({ index, row: item });
    }
    return {
      crudBinding,
      crudRef,
      goDetail,
      openEditDialog,
      deleteCert
    };
  }
});
</script>
<style lang="less">
.page-cert {
  .fs-container .box .inner > .body {
    background-color: #eee;
  }
  .cert-list {
    width: 100%;
    padding: 16px;

    .cert-item {
      margin: 10px;
      border: 1px solid #87b3b3;
      border-radius: 5px;
    }
  }

  .ant-pro-components-tag-select {
    :deep(.ant-pro-tag-select .ant-tag) {
      margin-right: 24px;
      padding: 0 8px;
      font-size: 14px;
    }
  }
  .ant-pro-pages-list-projects-cardList {
    margin-top: 24px;
    :deep(.ant-card-meta-title) {
      margin-bottom: 4px;
    }
    :deep(.ant-card-meta-description) {
      height: 44px;
      overflow: hidden;
      line-height: 22px;
    }
    .cardItemContent {
      display: flex;
      height: 20px;
      margin-top: 16px;
      margin-bottom: -4px;
      line-height: 20px;
      > span {
        flex: 1 1;
        color: rgba(0, 0, 0, 0.45);
        font-size: 12px;
      }
      :deep(.ant-pro-avatar-list) {
        flex: 0 1 auto;
      }
    }
  }
}
</style>
