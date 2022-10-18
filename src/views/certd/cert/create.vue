<template>
  <fs-page class="page-cert-create">
    <template #header>
      <div class="title">证书添加向导</div>
    </template>

    <a-steps :current="1">
      <a-step status="domain" title="输入域名" sub-title="支持通配符域名" description="可以多个域名打到一个证书里面" />
      <a-step status="challenge" title="域名校验" description="校验域名的所有权" />
      <a-step status="addition" title="附加选项" description="CSR信息" />
    </a-steps>

    <div v-if="current === 'domain'" class="step-domain">
      <a-form :model="form.challenge">
        <div class="input-row flex-row">
          <a-select
            v-model:value="form.cert.domains"
            size="large"
            mode="tags"
            placeholder="请输入域名，回车"
            :open="false"
          ></a-select>
          <div class="row-append"></div>
        </div>
        <div class="helper">
          支持泛域名（例如*.test.yourdomain.com）<br />
          支持多个域名打包到一张证书（输入一个域名后回车，再输下一个）
        </div>
        <a-form-item :wrapper-col="{ offset: 8, span: 16 }">
          <a-button type="primary" html-type="submit" @click="next('challenge')">下一步</a-button>
        </a-form-item>
      </a-form>
    </div>

    <div v-if="current === 'challenge'" class="step-challenge">
      <a-form :model="form.challenge">
        <a-form-item label="邮箱">
          <a-input v-model:value="form.challenge.email" />
        </a-form-item>
        <a-form-item label="验证类型">
          <fs-dict-select v-model:value="form.challenge.challengeType" :dict="challengeTypeDict"> </fs-dict-select>
        </a-form-item>
        <a-form-item label="DNS提供商">
          <fs-dict-select v-model:value="form.challenge.challengeDnsType" :dict="challengeDnsTypeDict"></fs-dict-select>
        </a-form-item>
      </a-form>
      <a-form-item :wrapper-col="{ offset: 8, span: 16 }">
        <a-button type="default" @click="prev('challenge')">上一步</a-button>
        <a-button type="primary" html-type="submit" @click="next('csr')">下一步</a-button>
      </a-form-item>
    </div>
  </fs-page>
</template>

<script>
import { defineComponent, ref, onMounted } from "vue";
import { dict } from "@fast-crud/fast-crud";
export default defineComponent({
  name: "CertdCertCreate",
  components: {},
  setup() {
    const form = ref({ cert: {}, challenge: {} });
    const current = ref("domain");
    const challengeTypeDict = dict({ data: [{ value: "dns", label: "DNS校验" }] });
    const challengeDnsTypeDict = dict({
      data: [
        { value: 1, label: "Aliyun" },
        { value: 2, label: "DnsPod" }
      ]
    });

    function prev(status) {
      current.value = stauts;
    }
    function next(status) {
      if (status === "challenge") {
        //检查domains
        if (form.value.cert.domains == null) {
        }
      }
    }
    return {
      form,
      current,
      challengeTypeDict,
      challengeDnsTypeDict
    };
  }
});
</script>
<style lang="less">
.page-cert-create {
  .domains-input {
    width: 100%;
    .input-row {
      width: 100%;
      .ant-select {
        flex: 1;
      }
      .row-append {
        padding-left: 10px;
      }
    }
  }
}
</style>
