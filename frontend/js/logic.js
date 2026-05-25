let steps = ['A', 'B', 'C', 'D'];
let currentIndex = 0;

function processCert() {
    if (currentIndex >= steps.length) {
        alert("所有任务已完成！");
        return;
    }

    let currentStep = steps[currentIndex];
    const time = new Date().toLocaleTimeString();

    // --- 模拟 PDF 要求的数据加固逻辑 ---
    // 逻辑：将 (步骤+时间) 进行 Base64 编码，再反转字符串
    let rawData = currentStep + "_" + time;
    let encryptedToken = btoa(rawData).split("").reverse().join("");

    // --- 模拟数据库操作 (LocalStorage) ---
    let db = JSON.parse(localStorage.getItem('smart_gis_db') || "[]");
    db.push({
        step: currentStep,
        time: time,
        token: encryptedToken,
        status: "Certified"
    });
    localStorage.setItem('smart_gis_db', JSON.stringify(db));

    // --- 更新 UI 界面 ---
    // 1. 列表变色并打勾
    let li = document.getElementById('li-' + currentStep);
    li.classList.add('completed');
    li.innerHTML = currentStep + ". " + getStepName(currentStep) + " ✓";

    // 2. 更新日志显示
    document.getElementById('log').innerText = JSON.stringify(db, null, 2);

    // 3. 更新提示文字
    currentIndex++;
    if (currentIndex < steps.length) {
        document.getElementById('current-task').innerText = "当前状态：请前往 " + steps[currentIndex] + " 点认证";
    } else {
        document.getElementById('current-task').innerText = "状态：印尼游客全程任务已达成！";
        document.getElementById('certifyBtn').disabled = true;
        document.getElementById('certifyBtn').style.background = "#ccc";
    }

    alert("认证成功！位置数据已加密同步至模拟 DB。");
}

function getStepName(id) {
    const names = { 'A': '身份核验', 'B': '文化活动', 'C': '材料费结算', 'D': '活动认证完成' };
    return names[id];
}

// 页面加载时自动清理旧数据，方便演示
window.onload = () => {
    localStorage.removeItem('smart_gis_db');
    console.log("演示开始：清理模拟数据库...");
};
