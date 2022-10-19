<template>
  <fs-page class="page-cert-detail">
    <div v-if="detailRef" class="cert-detail">
      <div class="cert-detail-left">
        <a-page-header title="证书申请" sub-title="可以将多个域名打到一个证书上">
          <template #extra>
            <a-button key="apply" type="primary">立即申请</a-button>
            <a-button key="edit" type="primary" @click="openCertEditDialog">编辑证书</a-button>
          </template>
          <a-descriptions>
            <a-descriptions-item label="域名"
              ><certd-domains :value="detailRef.cert.domains"></certd-domains
            ></a-descriptions-item>
            <a-descriptions-item label="邮箱">1810000000</a-descriptions-item>
            <a-descriptions-item label="其它信息">Hangzhou, Zhejiang</a-descriptions-item>
          </a-descriptions>

          <fs-form-wrapper ref="certEditDialogRef" v-bind="certEditDialogOptions" />
        </a-page-header>
        <a-page-header title="自动部署" sub-title="证书申请成功后，自动运行以下部署任务">
          <template #extra>
            <a-button key="add" type="primary">重新部署</a-button>
            <a-button key="add" type="primary">添加流程</a-button>
          </template>

          <div class="group-body cert-deploy">
            <a-card v-for="(deploy, index) of detailRef.deploy" :key="index" class="deploy-item" size="small">
              <template #title>
                <div class="deploy-name">
                  <template v-if="deploy._isEdit">
                    <a-input
                      v-model:value="deploy.title"
                      :validate-status="deploy.title ? '' : 'error'"
                      placeholder="请输入流程名称"
                      @keyup.enter="deployCloseEditMode(deploy)"
                      @blur="deployCloseEditMode(deploy)"
                    >
                      <template #suffix>
                        <CheckOutlined style="color: rgba(0, 0, 0, 0.45)" @click="deployCloseEditMode(deploy)" />
                      </template>
                    </a-input>
                  </template>
                  <template v-else>
                    <span @click="deployNameEdit"> <NodeIndexOutlined /> {{ deploy.title }}</span>
                    <EditOutlined class="ml-10 edit-icon" @click="deployOpenEditMode(deploy)" />
                  </template>
                </div>
              </template>
              <template #extra>
                <a-button type="danger" @click="deployDelete(deploy, index)">
                  <template #icon><DeleteOutlined /></template>
                </a-button>
              </template>
              <div class="task-list">
                <div v-for="(task, iindex) of deploy.tasks" :key="iindex" class="task-item-wrapper">
                  <a-button class="task-item" shape="round" @click="taskEdit(deploy, task, index)">
                    <ThunderboltOutlined />
                    {{ task.title }}
                  </a-button>
                  <ArrowRightOutlined class="task-next-icon" />
                </div>
                <div class="task-item-wrapper">
                  <a-button type="primary" class="task-item" shape="round" @click="taskAdd(deploy)">
                    <PlusOutlined />
                    添加新任务
                  </a-button>
                </div>
              </div>
            </a-card>
          </div>
        </a-page-header>
      </div>

      <div class="cert-log"></div>
    </div>
  </fs-page>
</template>

<script>
import { defineComponent, ref, onMounted, computed } from "vue";
import { useRoute } from "vue-router";
import CertdDomains from "/src/views/certd/cert/domains.vue";
import { useColumns } from "@fast-crud/fast-crud";
import createCrudOptions from "./crud";

function createFormOptionsFromCrudOptions() {
  const { buildFormOptions } = useColumns();
  //可以直接复用crud.js
  const { crudOptions } = createCrudOptions({});
  return buildFormOptions(crudOptions);
}

function useCertEditDialog() {
  const certEditDialogRef = ref();
  const certEditDialogOptions = ref(createFormOptionsFromCrudOptions());
  function openCertEditDialog() {
    certEditDialogRef.value.open(certEditDialogOptions.value);
  }
  return {
    certEditDialogRef,
    openCertEditDialog,
    certEditDialogOptions
  };
}

export default defineComponent({
  name: "CertdCertDetail",
  components: { CertdDomains },
  setup() {
    const route = useRoute();
    const id = route.query.id;
    const detailRef = ref();
    async function getDetail(id) {
      // const detail = await api.GetDetail(id);
      const detail = {
        cert: { domains: "*.yonsz.com,test.yonsz.net" },
        deploy: [
          {
            id: "xxxxx",
            title: "部署流程1",
            tasks: [
              {
                id: "xxxxx",
                title: "部署到阿里云AckIngress",
                type: "AliyunAckIngress"
              }
            ]
          }
        ],
        lastHistory: {
          result: "success",
          log: "xxxx"
        }
      };
      detailRef.value = detail;
    }
    getDetail(id);

    const tabActive = ref("deploy");
    return {
      detailRef,
      tabActive,
      ...useCertEditDialog()
    };
  }
});
</script>
<style lang="less">
.page-cert-detail {
  height: 100%;

  .ant-page-header-heading-title {
    font-size: 16px;
  }
  .ant-page-header-heading-sub-title {
    font-size: 12px;
    line-height: initial;
  }
  .content-box {
    padding: 20px;
  }
  .cert-detail {
    display: flex;
    .cert-detail-left {
      width: 70%;

      .cert-deploy {
        .task-item {
          .ant-card-bordered {
            border-color: #00aaaa;
          }
        }

        .flow-deploy {
          flex-shrink: 0;
          flex: 1;
          display: flex;
          flex-direction: column;

          .deploy-list {
            flex: 1;
            overflow-y: auto;

            .deploy-item {
              margin-bottom: 10px;
            }
          }

          // min-width:70%;
          .deploy-name {
            max-width: 300px;

            .edit-icon {
              color: #737070;
            }
          }
        }

        .task-list {
          display: flex;
          flex-direction: row;
          flex-wrap: wrap;
          justify-content: flex-start;
          align-items: center;

          > * {
            margin-bottom: 10px;
          }

          .task-item-wrapper {
            display: flex;
            flex-direction: row;
            flex-wrap: nowrap;
            justify-content: flex-start;
            align-items: center;
          }

          .task-item {
            //border: 1px solid #eee;
            //padding: 10px 20px;
            //border-radius: 20px;
          }

          .task-add-icon {
            font-size: 24px;
            margin-right: 10px;
          }

          .task-next-icon {
            margin-left: 10px;
            margin-right: 10px;
          }
        }
      }
    }
    .cert-log {
      width: 30%;
    }
  }
}
</style>
